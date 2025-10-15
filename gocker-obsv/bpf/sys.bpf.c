// bpf/sys.bpf.c
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

struct cg_key { __u64 cgid; __u32 type; __u32 pad; };

struct { __uint(type, BPF_MAP_TYPE_PERCPU_HASH); __uint(max_entries, 16384);
         __type(key, struct cg_key); __type(value, __u64); } cg_sys_cnt SEC(".maps");
struct { __uint(type, BPF_MAP_TYPE_PERCPU_HASH); __uint(max_entries, 16384);
         __type(key, struct cg_key); __type(value, __u64); } cg_sys_lat_ns SEC(".maps");

struct start_key { __u32 pid; __u32 sys; };
struct start_val { __u64 ts_ns; __u64 cgid; };
struct { __uint(type, BPF_MAP_TYPE_LRU_HASH); __uint(max_entries, 65536);
         __type(key, struct start_key); __type(value, struct start_val); } sys_enter_start SEC(".maps");

static __always_inline void load_cfg(__u32 *rate, __u32 *en, __u32 *lvl, __u64 *cgid) {
    __u32 k=0; struct cfg *c=bpf_map_lookup_elem(&cfg_map,&k);
    if (c){ *rate=c->sample_rate?c->sample_rate:1; *en=c->enable_filter; *lvl=c->target_level; *cgid=c->target_cgid; }
    else { *rate=SAMPLE_RATE?SAMPLE_RATE:1; *en=ENABLE_FILTER; *lvl=TARGET_LEVEL; *cgid=TARGET_CGID; }
}

SEC("tracepoint/raw_syscalls/sys_enter")
int tp_sys_enter(struct trace_event_raw_sys_enter* ctx)
{
    __u32 rate,en,lvl; __u64 cgid; load_cfg(&rate,&en,&lvl,&cgid);
    if (en) {
        __u64 anc = bpf_get_current_ancestor_cgroup_id((int)lvl);
        if (!(anc && anc == cgid)) return 0;
    }
    struct start_key k = { .pid = bpf_get_current_pid_tgid(), .sys = (__u32)ctx->id };
    struct start_val v = { .ts_ns = bpf_ktime_get_ns(), .cgid = bpf_get_current_cgroup_id() };
    bpf_map_update_elem(&sys_enter_start, &k, &v, BPF_ANY);
    return 0;
}

static __always_inline void add_u64(struct bpf_map *m, struct cg_key *k, __u64 delta)
{
    __u64 *val = bpf_map_lookup_elem(m, k);
    if (!val) { __u64 z=0; bpf_map_update_elem(m, k, &z, BPF_NOEXIST);
                val=bpf_map_lookup_elem(m,k); if(!val) return; }
    *val += delta;
}

SEC("tracepoint/raw_syscalls/sys_exit")
int tp_sys_exit(struct trace_event_raw_sys_exit* ctx)
{
    struct start_key k = { .pid = bpf_get_current_pid_tgid(), .sys = (__u32)ctx->id };
    struct start_val *sv = bpf_map_lookup_elem(&sys_enter_start, &k);
    if (!sv) return 0;

    __u64 now = bpf_ktime_get_ns();
    __u64 dt  = now > sv->ts_ns ? now - sv->ts_ns : 0;

    struct cg_key agg = { .cgid = sv->cgid, .type = k.sys };
    add_u64((struct bpf_map*)&cg_sys_cnt,    &agg, 1);
    add_u64((struct bpf_map*)&cg_sys_lat_ns, &agg, dt);

    bpf_map_delete_elem(&sys_enter_start, &k);
    return 0;
}
