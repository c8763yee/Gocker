#ifndef SCHED_MONITOR_H
#define SCHED_MONITOR_H

#ifdef __BPF__
typedef __u64 sm_u64;
typedef __u32 sm_u32;
typedef __s32 sm_s32;
#else
#include <stdint.h>
typedef uint64_t sm_u64;
typedef uint32_t sm_u32;
typedef int32_t  sm_s32;
#endif

#define DEFAULT_RUNTIME_THRESHOLD_NS (5ULL * 1000 * 1000)

enum evt_type {  // event type
    EVT_SWITCH  = 1,
    EVT_WAKEUP  = 2,
    EVT_RUNTIME = 3,
    EVT_MIGRATE = 4,
};

struct evt {     // record event
    sm_u64 ts;   // timestamp
    sm_u32 cpu;  // cpu id
    sm_u32 type; // event type

    sm_u32 pid;  // 等同於thread id
    sm_u32 tgid; // tgid = process ID（thread group leader 的 TID）
    sm_u64 cgroup_id;

    sm_s32 aux0;
    sm_s32 aux1;
    sm_u64 aux2;

    char comm[16];
};

struct cg_stat_val {
    sm_u64 runtime_ns;
    sm_u64 max_runtime_ns;
    sm_u64 ctx_switches;
    sm_u64 wakeups;
    sm_u64 migrations;
};

#undef sm_u64
#undef sm_u32
#undef sm_s32

#endif /* SCHED_MONITOR_H */
