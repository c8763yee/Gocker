// bpf/sched.bpf.c
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
enum sc_type { EVT_SWITCH_IN = 1, EVT_WAKEUP = 2 };
struct cg_key { __u64 cgid; __u32 type; __u32 pad; };

struct { __uint(type, BPF_MAP_TYPE_PERCPU_HASH); __uint(max_entries, 4096);
         __type(key, struct cg_key); __type(value, __u64); } cg_sched_cnt SEC(".maps");

struct { __uint(type, BPF_MAP_TYPE_LRU_HASH); __uint(max_entries, 65536);
         __type(key, __u32); __type(value, __u64); } pid_cgid SEC(".maps");

struct { __uint(type, BPF_MAP_TYPE_PERCPU_ARRAY); __uint(max_entries, 1);
         __type(key, __u32); __type(value, __u64); } per_cpu_cnt SEC(".maps");

static __always_inline void load_cfg(__u32 *rate, __u32 *en, __u32 *lvl, __u64 *cgid) {
    __u32 k=0; struct cfg *c=bpf_map_lookup_elem(&cfg_map,&k);
    if (c){ *rate=c->sample_rate?c->sample_rate:1; *en=c->enable_filter; *lvl=c->target_level; *cgid=c->target_cgid; }
    else { *rate=SAMPLE_RATE?SAMPLE_RATE:1; *en=ENABLE_FILTER; *lvl=TARGET_LEVEL; *cgid=TARGET_CGID; }
}
static __always_inline bool in_target_subtree_current(void) {
    __u32 rate,en,lvl; __u64 cgid; load_cfg(&rate,&en,&lvl,&cgid);
    if (!en) return true;
    __u64 anc = bpf_get_current_ancestor_cgroup_id((int)lvl);
    return anc && anc == cgid;
}
static __always_inline bool pass_sample(void) {
    __u32 rate,en,lvl; __u64 cgid; load_cfg(&rate,&en,&lvl,&cgid);
    __u32 k=0; __u64 *pc=bpf_map_lookup_elem(&per_cpu_cnt,&k);
    if (!pc) return true; *pc += 1; return (*pc % rate) == 0;
}
static __always_inline int bump_sched(__u64 cgid, __u32 type) {
    if (!pass_sample()) return 0;
    struct cg_key key={.cgid=cgid,.type=type};
    __u64 *val=bpf_map_lookup_elem(&cg_sched_cnt,&key);
    if (!val){ __u64 z=0; bpf_map_update_elem(&cg_sched_cnt,&key,&z,BPF_NOEXIST);
               val=bpf_map_lookup_elem(&cg_sched_cnt,&key); if(!val) return 0; }
    *val += 1; return 0;
}

SEC("tracepoint/sched/sched_switch")
int tp_sched_switch(struct trace_event_raw_sched_switch* ctx)
{
    if (in_target_subtree_current()) {
        __u32 next = ctx->next_pid;
        __u64 cur_cgid = bpf_get_current_cgroup_id();
        bpf_map_update_elem(&pid_cgid, &next, &cur_cgid, BPF_ANY);
        bump_sched(cur_cgid, EVT_SWITCH_IN);
    }
    return 0;
}

SEC("tracepoint/sched/sched_wakeup")
int tp_sched_wakeup(struct trace_event_raw_sched_wakeup_template* ctx)
{
    __u32 pid = ctx->pid;
    __u64 *cgid = bpf_map_lookup_elem(&pid_cgid, &pid);
    if (!cgid) return 0;
    return bump_sched(*cgid, EVT_WAKEUP);
}

SEC("tracepoint/sched/sched_process_exit")
int tp_sched_exit(struct trace_event_raw_sched_process_template* ctx)
{
    __u32 pid = bpf_get_current_pid_tgid();
    bpf_map_delete_elem(&pid_cgid, &pid);
    return 0;
}
