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

enum evt_type {
    EVT_SWITCH  = 1,
    EVT_WAKEUP  = 2,
    EVT_RUNTIME = 3,
    EVT_MIGRATE = 4,
};

struct evt {
    sm_u64 ts;
    sm_u32 cpu;
    sm_u32 type;

    sm_u32 pid;
    sm_u32 tgid;
    sm_u64 cgroup_id;

    sm_s32 aux0;
    sm_s32 aux1;
    sm_u64 aux2;

    char comm[16];
};

struct pid_stat_key {
    sm_u64 cgid;
    sm_u32 pid;
    sm_u32 pad;
};

struct pid_stat_val {
    sm_u64 runtime_ns;
    sm_u64 ctx_switches;
    sm_u64 wakeups;
    sm_u64 migrations;
};

struct cg_stat_val {
    sm_u64 runtime_ns;
    sm_u64 ctx_switches;
    sm_u64 wakeups;
    sm_u64 migrations;
};

#undef sm_u64
#undef sm_u32
#undef sm_s32

#endif /* SCHED_MONITOR_H */
