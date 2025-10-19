// bpf/cpu.bpf.c
#include "vmlinux.h"
#include <bpf/bpf_helpers.h>
#include <bpf/bpf_tracing.h>
#include <bpf/bpf_core_read.h>

char LICENSE[] SEC("license") = "Dual BSD/GPL";

const volatile __u32 SAMPLE_RATE   = 1;
const volatile __u32 ENABLE_FILTER = 1;
const volatile __u32 TARGET_LEVEL  = 1;
const volatile __u64 TARGET_CGID   = 0;

struct cfg { __u32 sample_rate, enable_filter, target_level; __u64 target_cgid; };
struct { __uint(type, BPF_MAP_TYPE_ARRAY); __uint(max_entries, 1);
         __type(key, __u32); __type(value, struct cfg); } cfg_map SEC(".maps");

enum cpu_type { CPU_RUNTIME = 1, CPU_WAIT = 2, CPU_IOWAIT = 3 };
struct cg_key { __u64 cgid; __u32 type; __u32 pad; };

// 說明：cg_cpu_ns 用於累加各 cgroup 的 runtime/wait/iowait（以奈秒為單位）
struct {
    __uint(type, BPF_MAP_TYPE_PERCPU_HASH);
    __uint(max_entries, 4096);
    __type(key, struct cg_key);
    __type(value, __u64);
} cg_cpu_ns SEC(".maps");

struct {
    __uint(type, BPF_MAP_TYPE_LRU_HASH);
    __uint(max_entries, 65536);
    __type(key, __u32);
    __type(value, __u64);
} pid_cgid SEC(".maps");

struct {
    __uint(type, BPF_MAP_TYPE_PERCPU_ARRAY);
    __uint(max_entries, 1);
    __type(key, __u32);
    __type(value, __u64);
} per_cpu_cnt SEC(".maps");

// 說明：load_cfg 與 pass_sample 與其他模組共享邏輯，透過 cfg_map 調整熱更新參數
static __always_inline void load_cfg(__u32 *rate, __u32 *en, __u32 *lvl, __u64 *cgid) {
    __u32 k = 0;
    struct cfg *c = bpf_map_lookup_elem(&cfg_map, &k);
    if (c) { *rate = c->sample_rate ? c->sample_rate : 1; *en = c->enable_filter; *lvl = c->target_level; *cgid = c->target_cgid; }
    else   { *rate = SAMPLE_RATE ? SAMPLE_RATE : 1;       *en = ENABLE_FILTER;     *lvl = TARGET_LEVEL;     *cgid = TARGET_CGID; }
}

static __always_inline bool in_target_subtree_current(void) {
    __u32 rate, en, lvl; __u64 cgid;
    load_cfg(&rate, &en, &lvl, &cgid);
    if (!en) {
        return true;
    }
    __u64 anc = bpf_get_current_ancestor_cgroup_id((int)lvl);
    return anc && anc == cgid;
}

static __always_inline bool pass_sample(void) {
    __u32 rate, en, lvl; __u64 cgid;
    load_cfg(&rate, &en, &lvl, &cgid);
    __u32 k = 0;
    __u64 *pc = bpf_map_lookup_elem(&per_cpu_cnt, &k);
    if (!pc) {
        return true;
    }
    *pc += 1;
    if (rate <= 1) {
        return true;
    }
    return (*pc % rate) == 0;
}

static __always_inline void remember_pid(__u32 pid, __u64 cgid) {
    if (!pid || !cgid) {
        return;
    }
    bpf_map_update_elem(&pid_cgid, &pid, &cgid, BPF_ANY);
}

// 說明：若 pid 已存在對應 cgroup id，可透過此函式回傳給後續計算使用
static __always_inline bool pid_to_cgid(__u32 pid, __u64 *cgid) {
    __u64 *val = bpf_map_lookup_elem(&pid_cgid, &pid);
    if (!val) {
        return false;
    }
    *cgid = *val;
    return true;
}

static __always_inline int add_cpu_ns(__u64 cgid, __u32 type, __u64 delta) {
    if (!cgid || !delta) {
        return 0;
    }
    struct cg_key key = { .cgid = cgid, .type = type };
    __u64 *val = bpf_map_lookup_elem(&cg_cpu_ns, &key);
    if (!val) {
        __u64 zero = 0;
        bpf_map_update_elem(&cg_cpu_ns, &key, &zero, BPF_NOEXIST);
        val = bpf_map_lookup_elem(&cg_cpu_ns, &key);
        if (!val) {
            return 0;
        }
    }
    *val += delta;
    return 0;
}

SEC("tracepoint/sched/sched_switch")
int tp_sched_switch_cpu(struct trace_event_raw_sched_switch *ctx)
{
    if (!in_target_subtree_current()) {
        return 0;
    }
    __u64 cur_cgid = bpf_get_current_cgroup_id();
    remember_pid((__u32)ctx->prev_pid, cur_cgid);
    return 0;
}

SEC("tracepoint/sched/sched_process_exit")
int tp_sched_exit_cpu(struct trace_event_raw_sched_process_template *ctx)
{
    __u32 pid = bpf_get_current_pid_tgid();
    bpf_map_delete_elem(&pid_cgid, &pid);
    return 0;
}

SEC("tracepoint/sched/sched_stat_runtime")
int tp_sched_stat_runtime(struct trace_event_raw_sched_stat_runtime *ctx)
{
    __u32 pid = (__u32)ctx->pid;
    __u64 cgid = 0;
    if (in_target_subtree_current()) {
        cgid = bpf_get_current_cgroup_id();
        remember_pid(pid, cgid);
    } else {
        if (!pid_to_cgid(pid, &cgid)) {
            return 0;
        }
    }
    if (!pass_sample()) {
        return 0;
    }
    return add_cpu_ns(cgid, CPU_RUNTIME, ctx->runtime);
}

// 說明：handle_delay_event 將 wait/iowait 延遲歸戶到對應 cgroup
static __always_inline int handle_delay_event(__u32 pid, __u64 delay, __u32 type)
{
    __u64 cgid = 0;
    if (!pid_to_cgid(pid, &cgid)) {
        return 0;
    }
    if (!pass_sample()) {
        return 0;
    }
    return add_cpu_ns(cgid, type, delay);
}

SEC("tracepoint/sched/sched_stat_wait")
int tp_sched_stat_wait(struct trace_event_raw_sched_stat_template *ctx)
{
    return handle_delay_event((__u32)ctx->pid, ctx->delay, CPU_WAIT);
}

SEC("tracepoint/sched/sched_stat_iowait")
int tp_sched_stat_iowait(struct trace_event_raw_sched_stat_template *ctx)
{
    return handle_delay_event((__u32)ctx->pid, ctx->delay, CPU_IOWAIT);
}
