#include "vmlinux.h"
#include "sched_monitor.h"
#include <bpf/bpf_helpers.h>
#include <bpf/bpf_tracing.h>
#include <bpf/bpf_core_read.h>

char LICENSE[] SEC("license") = "GPL";

// 只紀錄執行時間超過 5ms 的事件
// user-space 可以調整以下兩者
const volatile __u64 runtime_event_threshold_ns = DEFAULT_RUNTIME_THRESHOLD_NS;  // 觸發事件的時間閾值
const volatile __u8 filter_enabled;  // cgroup過濾功能的開關

/* allow-list of cgroup IDs we care about */
struct {
    __uint(type, BPF_MAP_TYPE_HASH);
    __type(key, __u64);  // key : cgroup id  
    __type(value, __u8);  // bool allowed : 
    __uint(max_entries, 8192);  // 最多允許 8192 筆
} allowed_cgroups SEC(".maps");

// 統計每個 cgroup 數據
struct {
    __uint(type, BPF_MAP_TYPE_HASH);
    __type(key, __u64); // cgroup_id -> struct cp_stat_val
    __type(value, struct cg_stat_val);
    __uint(max_entries, 8192);
} cg_stats SEC(".maps");

// ring buffer 裝事件
struct {
    __uint(type, BPF_MAP_TYPE_RINGBUF);
    __uint(max_entries, 1 << 24); // 16MB ring buffer; adjust to your load
} events_rb SEC(".maps");



// 取得目前執行的 process (task) 所屬的 cgroup ID
static __always_inline __u64 get_cgid(void)
{
    // cgroup v2 only. If v1, this will be 0.
    return bpf_get_current_cgroup_id();
}


// 決定目前事件是否屬於被監控的cgroup
static __always_inline int allow_cgroup(__u64 cgid)
{
    if (!filter_enabled)
        return 1;
    return bpf_map_lookup_elem(&allowed_cgroups, &cgid) ? 1 : 0;
}

static __always_inline void emit_event(struct evt *e, enum evt_type type, __u64 cgid)
{
    e->type = type;  // 設定event type
    e->ts = bpf_ktime_get_ns();  // 取得timestamp (64bits)
    e->cpu = bpf_get_smp_processor_id();  // 目前執行的邏輯 cpu id 
    e->cgroup_id = cgid;  // // 呼叫者傳入的 cgroup id
    __u64 id = bpf_get_current_pid_tgid();            // 64bit 聚合：高32=TGID(=process id), 低32=TID(=thread id)
    e->pid = id & 0xffffffffu;                        // 低 32-bit = TID（Linux「pid」在 BPF 內常指 TID）
    e->tgid = id >> 32;                               // 高 32-bit = TGID（thread group id, 也就是傳統意義的「程序 PID」）
    bpf_get_current_comm(&e->comm, sizeof(e->comm));  // 拷貝當前 task 的 comm（最多 15 字元 + '\0'）
}

// 找出對應 cgroup 的統計記錄，如沒有則初始化，最後累加 runtime
static __always_inline void cgstats_add_runtime(__u64 cgid, __u64 delta)
{
    struct cg_stat_val *v, zero = {};  // v 指向目前這個cgroup的統計資料, zero = {}用於初始化新cgroup entry(所有欄位歸零)
    v = bpf_map_lookup_elem(&cg_stats, &cgid);
    if (!v) { //若這個 cgroup 還沒出現在統計表中
        bpf_map_update_elem(&cg_stats, &cgid, &zero, BPF_NOEXIST);  // 則先用「全為 0」的結構插入新 entry
        v = bpf_map_lookup_elem(&cg_stats, &cgid);  // 然後再重新查找一次（拿到指標）
        if (!v) return;
    }
    __sync_fetch_and_add(&v->runtime_ns, delta);  //將這次執行時間差 (delta) 累加到該 cgroup 的總 runtime
}

// per-cgroup 聚合（aggregation）
// 把「以 cgroup ID 為 key」的統計值（context switches / wakeups / migrations）累加
static __always_inline void cgstats_inc(__u64 cgid, int which)  // which 指的是事件種類
{
    struct cg_stat_val *v, zero = {};
    v = bpf_map_lookup_elem(&cg_stats, &cgid);
    if (!v) {     // 若這個cgroup 還沒出現過
        bpf_map_update_elem(&cg_stats, &cgid, &zero, BPF_NOEXIST);  // 插入一筆新的{ cgid -> 全部欄位歸零}
        v = bpf_map_lookup_elem(&cg_stats, &cgid);  // 再重新查找
        if (!v) return;
    }
    // 根據事件種類決定要加到哪個統計項目
    // 利用_sync_fetch_and_add()做 atomic 加法統計事件
    if (which == 0)  
        __sync_fetch_and_add(&v->ctx_switches, 1);
    else if (which == 1)
        __sync_fetch_and_add(&v->wakeups, 1);
    else if (which == 2)
        __sync_fetch_and_add(&v->migrations, 1);
}


// ---- Tracepoints ----

SEC("tracepoint/sched/sched_switch")  // context switch 
int tp_sched_switch(struct trace_event_raw_sched_switch *ctx)
{
    __u64 cgid = get_cgid();  // get container id 

    if (!allow_cgroup(cgid))
        return 0;

    struct evt *e = bpf_ringbuf_reserve(&events_rb, sizeof(*e), 0);
    if (!e)
        return 0;
    emit_event(e, EVT_SWITCH, cgid);


    // For printing/verification
    e->aux0 = ctx->prev_pid;
    e->aux1 = ctx->next_pid;
    e->aux2 = 0;


    // Per-cgroup aggregation: count context switches for the *current* (next) task
    cgstats_inc(cgid, 0);


    bpf_ringbuf_submit(e, 0);
    return 0;
}


SEC("tracepoint/sched/sched_wakeup")  // process 從sleep → ready
int tp_sched_wakeup(void *raw_ctx)
{
    __u64 cgid = get_cgid();

    if (!allow_cgroup(cgid))
        return 0;

    const struct trace_event_raw_sched_wakeup_template *ctx = raw_ctx;
    struct evt *e = bpf_ringbuf_reserve(&events_rb, sizeof(*e), 0);
    if (!e)
        return 0;
    emit_event(e, EVT_WAKEUP, cgid);


    e->aux0 = ctx->target_cpu;
    e->aux1 = ctx->prio;
    e->aux2 = 0;


    // Attribute to current task's cgroup (the waker). This is a simplification.
    cgstats_inc(cgid, 1);


    bpf_ringbuf_submit(e, 0);
    return 0;
}
/*
每次任務被切換出（switch out）時，就會：

計算該任務在 CPU 上跑了多久（delta_ns）；

找出它所屬的 cgroup；

將這段時間累加到對應的 cg_stats[cgid].runtime_ns。

這樣，cg_stats map 內就會持續累積「每個 cgroup 總共佔用 CPU 的時間」。
*/
SEC("tracepoint/sched/sched_stat_runtime")  //
int tp_sched_stat_runtime(struct trace_event_raw_sched_stat_runtime *ctx)
{
    __u64 cgid = get_cgid();

    if (!allow_cgroup(cgid))
        return 0;

    if (runtime_event_threshold_ns &&
        ctx->runtime < runtime_event_threshold_ns)  // 只有當次 ctx->runtime（ns）≥ 門檻才繼續, 在 .rodata 可由 user space 設定的參數，用於抑制過短事件。
        return 0;


    // Update per-cgroup runtime aggregation first
    cgstats_add_runtime(cgid, ctx->runtime);


    // Also emit an event for verification
    struct evt *e = bpf_ringbuf_reserve(&events_rb, sizeof(*e), 0);
    if (!e)
        return 0;
    emit_event(e, EVT_RUNTIME, cgid);
    e->aux0 = 0;
    e->aux1 = 0;
    e->aux2 = ctx->runtime; // ns
    bpf_ringbuf_submit(e, 0);
    return 0;
}


SEC("tracepoint/sched/sched_migrate_task")  // process migration
int tp_sched_migrate_task(struct trace_event_raw_sched_migrate_task *ctx)
{
    __u64 cgid = get_cgid();

    if (!allow_cgroup(cgid))
        return 0;

    struct evt *e = bpf_ringbuf_reserve(&events_rb, sizeof(*e), 0);
    if (!e)
        return 0;
    emit_event(e, EVT_MIGRATE, cgid);


    e->aux0 = ctx->dest_cpu;
    e->aux1 = ctx->orig_cpu;
    e->aux2 = 0;


    cgstats_inc(cgid, 2);


    bpf_ringbuf_submit(e, 0);
    return 0;
}
