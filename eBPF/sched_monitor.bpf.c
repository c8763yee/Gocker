#include "vmlinux.h"
#include "sched_monitor.h"
#include <bpf/bpf_helpers.h>
#include <bpf/bpf_tracing.h>
#include <bpf/bpf_core_read.h>
#include <stdbool.h>

char LICENSE[] SEC("license") = "GPL";

// Event model pushed to ringbuf 

struct {
    __uint(type, BPF_MAP_TYPE_HASH);
    __type(key, __u64); // cgroup_id
    __type(value, struct cg_stat_val);
    __uint(max_entries, 8192);
} cg_stats SEC(".maps");  

struct {
    __uint(type, BPF_MAP_TYPE_RINGBUF);
    __uint(max_entries, 1 << 24); // 16MB ring buffer; adjust to your load
} events_rb SEC(".maps");

struct {
    __uint(type, BPF_MAP_TYPE_HASH);
    __type(key, struct pid_stat_key);
    __type(value, struct pid_stat_val);
    __uint(max_entries, 32768);
} pid_stats SEC(".maps");

const volatile __u64 target_cgid = 0;

static __always_inline __u64 get_cgid(void)
{
    // cgroup v2 only. If v1, this will be 0.
    return bpf_get_current_cgroup_id();
}

static __always_inline bool cgid_allowed(__u64 cgid)
{
    return target_cgid == 0 || cgid == target_cgid;
}

static __always_inline void emit_event(struct evt *e, enum evt_type type, __u64 cgid)
{
    e->type = type;
    e->ts = bpf_ktime_get_ns();
    e->cpu = bpf_get_smp_processor_id();
    e->cgroup_id = cgid;
    __u64 id = bpf_get_current_pid_tgid();
    e->pid = id & 0xffffffffu;
    e->tgid = id >> 32;
    bpf_get_current_comm(&e->comm, sizeof(e->comm));
}

static __always_inline void cgstats_add_runtime(__u64 cgid, __u64 delta)
{
    struct cg_stat_val *v, zero = {};
    v = bpf_map_lookup_elem(&cg_stats, &cgid);
    if (!v) {
        bpf_map_update_elem(&cg_stats, &cgid, &zero, BPF_NOEXIST);
        v = bpf_map_lookup_elem(&cg_stats, &cgid);
        if (!v) return;
    }
    __sync_fetch_and_add(&v->runtime_ns, delta);
}


static __always_inline void cgstats_inc(__u64 cgid, int which)
{
    struct cg_stat_val *v, zero = {};
    v = bpf_map_lookup_elem(&cg_stats, &cgid);
    if (!v) {
        bpf_map_update_elem(&cg_stats, &cgid, &zero, BPF_NOEXIST);
        v = bpf_map_lookup_elem(&cg_stats, &cgid);
        if (!v) return;
    }
    if (which == 0)
        __sync_fetch_and_add(&v->ctx_switches, 1);
    else if (which == 1)
        __sync_fetch_and_add(&v->wakeups, 1);
    else if (which == 2)
        __sync_fetch_and_add(&v->migrations, 1);
}

static __always_inline void pidstats_add_runtime(__u64 cgid, __u32 pid, __u64 delta)
{
    struct pid_stat_key key = { .cgid = cgid, .pid = pid };
    struct pid_stat_val *v, zero = {};

    v = bpf_map_lookup_elem(&pid_stats, &key);
    if (!v) {
        bpf_map_update_elem(&pid_stats, &key, &zero, BPF_NOEXIST);
        v = bpf_map_lookup_elem(&pid_stats, &key);
        if (!v)
            return;
    }
    __sync_fetch_and_add(&v->runtime_ns, delta);
}

static __always_inline void pidstats_inc(__u64 cgid, __u32 pid, int which)
{
    struct pid_stat_key key = { .cgid = cgid, .pid = pid };
    struct pid_stat_val *v, zero = {};

    v = bpf_map_lookup_elem(&pid_stats, &key);
    if (!v) {
        bpf_map_update_elem(&pid_stats, &key, &zero, BPF_NOEXIST);
        v = bpf_map_lookup_elem(&pid_stats, &key);
        if (!v)
            return;
    }

    if (which == 0)
        __sync_fetch_and_add(&v->ctx_switches, 1);
    else if (which == 1)
        __sync_fetch_and_add(&v->wakeups, 1);
    else if (which == 2)
        __sync_fetch_and_add(&v->migrations, 1);
}


// ---- Tracepoints ----

SEC("tracepoint/sched/sched_switch")
int tp_sched_switch(struct trace_event_raw_sched_switch *ctx)
{
    __u64 cgid = get_cgid();

    if (!cgid_allowed(cgid))
        return 0;

    struct evt *e = bpf_ringbuf_reserve(&events_rb, sizeof(*e), 0);
    if (!e)
        return 0;
    emit_event(e, EVT_SWITCH, cgid);
    pidstats_inc(cgid, e->pid, 0);


    // For printing/verification
    e->aux0 = ctx->prev_pid;
    e->aux1 = ctx->next_pid;
    e->aux2 = 0;


    // Per-cgroup aggregation: count context switches for the *current* (next) task
    cgstats_inc(cgid, 0);


    bpf_ringbuf_submit(e, 0);
    return 0;
}


SEC("tracepoint/sched/sched_wakeup")
int tp_sched_wakeup(void *raw_ctx)
{
    const struct trace_event_raw_sched_wakeup_template *ctx = raw_ctx;
    __u64 cgid = get_cgid();

    if (!cgid_allowed(cgid))
        return 0;

    struct evt *e = bpf_ringbuf_reserve(&events_rb, sizeof(*e), 0);
    if (!e)
        return 0;
    emit_event(e, EVT_WAKEUP, cgid);
    pidstats_inc(cgid, e->pid, 1);


    e->aux0 = ctx->target_cpu;
    e->aux1 = ctx->prio;
    e->aux2 = 0;


    // Attribute to current task's cgroup (the waker). This is a simplification.
    cgstats_inc(cgid, 1);


    bpf_ringbuf_submit(e, 0);
    return 0;
}


SEC("tracepoint/sched/sched_stat_runtime")
int tp_sched_stat_runtime(struct trace_event_raw_sched_stat_runtime *ctx)
{
    __u64 cgid = get_cgid();

    if (!cgid_allowed(cgid))
        return 0;


    // Update per-cgroup runtime aggregation first
    cgstats_add_runtime(cgid, ctx->runtime);


    // Also emit an event for verification
    struct evt *e = bpf_ringbuf_reserve(&events_rb, sizeof(*e), 0);
    if (!e)
        return 0;
    emit_event(e, EVT_RUNTIME, cgid);
    pidstats_add_runtime(cgid, e->pid, ctx->runtime);
    e->aux0 = 0;
    e->aux1 = 0;
    e->aux2 = ctx->runtime; // ns
    bpf_ringbuf_submit(e, 0);
    return 0;
}


SEC("tracepoint/sched/sched_migrate_task")
int tp_sched_migrate_task(struct trace_event_raw_sched_migrate_task *ctx)
{
    __u64 cgid = get_cgid();

    if (!cgid_allowed(cgid))
        return 0;

    struct evt *e = bpf_ringbuf_reserve(&events_rb, sizeof(*e), 0);
    if (!e)
        return 0;
    emit_event(e, EVT_MIGRATE, cgid);
    pidstats_inc(cgid, e->pid, 2);


    e->aux0 = ctx->dest_cpu;
    e->aux1 = ctx->orig_cpu;
    e->aux2 = 0;


    cgstats_inc(cgid, 2);


    bpf_ringbuf_submit(e, 0);
    return 0;
}
