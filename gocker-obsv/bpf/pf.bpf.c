// bpf/pf.bpf.c
#include "vmlinux.h"
#include <bpf/bpf_helpers.h>
#include <bpf/bpf_tracing.h>
#include <bpf/bpf_core_read.h>

char LICENSE[] SEC("license") = "Dual BSD/GPL";

// ---- fallback defaults (.rodata) ----
const volatile __u32 SAMPLE_RATE   = 1;
const volatile __u32 ENABLE_FILTER = 1;
const volatile __u32 TARGET_LEVEL  = 1;
const volatile __u64 TARGET_CGID   = 0;

// ---- shared config map (runtime override) ----
struct cfg { __u32 sample_rate, enable_filter, target_level; __u64 target_cgid; };
struct { __uint(type, BPF_MAP_TYPE_ARRAY); __uint(max_entries, 1);
         __type(key, __u32); __type(value, struct cfg); } cfg_map SEC(".maps");

enum pf_type { PF_USER = 1, PF_KERNEL = 2 };
struct cg_key { __u64 cgid; __u32 type; __u32 pad; };

struct {
    __uint(type, BPF_MAP_TYPE_PERCPU_HASH);
    __uint(max_entries, 4096);
    __type(key, struct cg_key);
    __type(value, __u64);
} cg_pf_cnt SEC(".maps");

struct { __uint(type, BPF_MAP_TYPE_PERCPU_ARRAY); __uint(max_entries, 1);
         __type(key, __u32); __type(value, __u64); } per_cpu_cnt SEC(".maps");

static __always_inline void load_cfg(__u32 *rate, __u32 *en, __u32 *lvl, __u64 *cgid) {
    __u32 k = 0;
    struct cfg *c = bpf_map_lookup_elem(&cfg_map, &k);
    if (c) { *rate = c->sample_rate?c->sample_rate:1; *en=c->enable_filter; *lvl=c->target_level; *cgid=c->target_cgid; }
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
static __always_inline int bump_pf(__u32 type)
{
    if (!in_target_subtree_current()) return 0;
    if (!pass_sample()) return 0;
    struct cg_key key = { .cgid = bpf_get_current_cgroup_id(), .type = type };
    __u64 *val = bpf_map_lookup_elem(&cg_pf_cnt, &key);
    if (!val) { __u64 z=0; bpf_map_update_elem(&cg_pf_cnt,&key,&z,BPF_NOEXIST);
                val = bpf_map_lookup_elem(&cg_pf_cnt,&key); if(!val) return 0; }
    *val += 1; return 0;
}

SEC("tracepoint/exceptions/page_fault_user")
int tp_page_fault_user(void *ctx) { return bump_pf(PF_USER); }

SEC("tracepoint/exceptions/page_fault_kernel")
int tp_page_fault_kernel(void *ctx) { return bump_pf(PF_KERNEL); }
