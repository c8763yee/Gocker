# Minimal Makefile for libbpf CO-RE example
# Requires: clang/llvm, bpftool, pkg-config, libbpf (or build from submodule)


BPF_CLANG ?= clang
BPF_LLVM_STRIP ?= llvm-strip
CC ?= cc
CFLAGS := -O2 -g -Wall -Werror $(CFLAGS)
BPF_CFLAGS ?= -I.


# If libbpf is installed system-wide:
LIBS := $(shell pkg-config --libs libbpf 2>/dev/null || echo -lbpf)
INCS := $(shell pkg-config --cflags libbpf 2>/dev/null)


# Fallback include path for vmlinux.h if you keep it locally; optional if you rely on CO-RE + BTF from /sys.
INCS += -I.


BPF_OBJ = sched_monitor.bpf.o
SKEL_H = sched_monitor.skel.h
USER = main
TARGET = ebpf-sched-monitor


.PHONY: all clean


all: $(TARGET)


$(BPF_OBJ): sched_monitor.bpf.c sched_monitor.h
	$(BPF_CLANG) -target bpf -D__TARGET_ARCH_$(shell uname -m | sed 's/x86_64/x86/;s/aarch64/arm64/') \
		$(BPF_CFLAGS) -O2 -g -c $< -o $@
	$(BPF_LLVM_STRIP) -g $@


$(SKEL_H): $(BPF_OBJ)
	bpftool gen skeleton $< > $@


$(TARGET): $(USER).c $(SKEL_H) sched_monitor.h
	$(CC) $(CFLAGS) $(INCS) $< -o $@ $(LIBS)


clean:
	rm -f $(BPF_OBJ) $(SKEL_H) $(TARGET)
