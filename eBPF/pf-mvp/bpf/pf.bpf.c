// bpf/pf.bpf.c
// Build: make -C bpf
// 需要：clang/llvm、bpftool（用來產生 vmlinux.h）、kernel BTF、root 權限載入

#include "vmlinux.h"
#include <bpf/bpf_helpers.h>
#include <bpf/bpf_tracing.h>
#include <bpf/bpf_core_read.h>

char LICENSE[] SEC("license") = "Dual BSD/GPL";

// === 可由 userspace 以 RewriteConstants 覆寫的參數（.rodata） ===
const volatile __u32 SAMPLE_RATE   = 1;   // 每 N 次 fault 才送一次 event；1 = 全送
const volatile __u32 ENABLE_FILTER = 0;   // 1=啟用 cgroup 過濾
const volatile __u32 TARGET_LEVEL  = 0;   // gocker 路徑相對 /sys/fs/cgroup 的層級（gocker=1）
const volatile __u64 TARGET_CGID   = 0;   // 目標祖先 cgroup 的 cgroup_id (inode)

// === ring buffer event ===
struct event {
    __u64 ts_ns;
    __u32 type;        // 1=user, 2=kernel
    __u32 pid;
    __u32 tgid;
    __u64 cgroup_id;
    char  comm[16];
};

// ring buffer map
struct {
    __uint(type, BPF_MAP_TYPE_RINGBUF);
    __uint(max_entries, 1 << 24); // 16 MiB
} events SEC(".maps");

// 簡單抽樣計數器（per-CPU）
struct {
    __uint(type, BPF_MAP_TYPE_PERCPU_ARRAY);
    __uint(max_entries, 1);
    __type(key, __u32);
    __type(value, __u64);
} per_cpu_cnt SEC(".maps");

// 是否通過 cgroup 過濾
static __always_inline bool pass_cgroup_filter(void)
{
    if (!ENABLE_FILTER)
        return true;

    // 取得目前 task 在 cgroup v2 下「第 TARGET_LEVEL 層祖先」的 cgroup_id
    __u64 anc = bpf_get_current_ancestor_cgroup_id((int)TARGET_LEVEL);
    if (!anc)
        return false; // 取得失敗時保守丟棄
    return anc == TARGET_CGID;
}

static __always_inline int handle_fault(__u32 type)
{
    if (!pass_cgroup_filter())
        return 0;

    // 抽樣
    __u32 key = 0;
    __u64 *cnt = bpf_map_lookup_elem(&per_cpu_cnt, &key);
    __u64 val = 0;
    if (cnt) {
        val = *cnt + 1;
        *cnt = val;
        __u32 rate = SAMPLE_RATE ? SAMPLE_RATE : 1;
        if ((val % rate) != 0)
            return 0;
    }

    // 構造事件
    struct event *e = bpf_ringbuf_reserve(&events, sizeof(*e), 0);
    if (!e) return 0;

    e->ts_ns     = bpf_ktime_get_ns();
    e->type      = type;
    __u64 pidtgid = bpf_get_current_pid_tgid();
    e->pid       = (__u32)pidtgid;
    e->tgid      = (__u32)(pidtgid >> 32);
    e->cgroup_id = bpf_get_current_cgroup_id();
    bpf_get_current_comm(&e->comm, sizeof(e->comm));

    bpf_ringbuf_submit(e, 0);
    return 0;
}

SEC("tracepoint/exceptions/page_fault_user")
int tp_page_fault_user(void *ctx) { return handle_fault(1); }

SEC("tracepoint/exceptions/page_fault_kernel")
int tp_page_fault_kernel(void *ctx) { return handle_fault(2); }
