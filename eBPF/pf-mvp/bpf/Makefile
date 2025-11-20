# bpf/Makefile
# 依 CPU 架構設定 __TARGET_ARCH_*
ARCH := $(shell uname -m)
ifeq ($(ARCH),x86_64)
  TARGET_ARCH := x86
else ifeq ($(ARCH),aarch64)
  TARGET_ARCH := arm64
else
  $(error Unsupported arch $(ARCH); set TARGET_ARCH manually)
endif

CLANG ?= clang
BPF_CFLAGS := -O2 -g -target bpf -D__TARGET_ARCH_$(TARGET_ARCH) -Wall -Wno-unused-value -Wno-pointer-sign -Wno-compare-distinct-pointer-types

all: vmlinux.h pf.bpf.o

vmlinux.h:
	@if [ ! -e /sys/kernel/btf/vmlinux ]; then \
	  echo "ERROR: /sys/kernel/btf/vmlinux not found (install kernel BTF / debuginfo)"; exit 1; \
	fi
	@which bpftool >/dev/null 2>&1 || (echo "ERROR: bpftool not found (apt install bpftool)"; exit 1)
	bpftool btf dump file /sys/kernel/btf/vmlinux format c > vmlinux.h

pf.bpf.o: pf.bpf.c vmlinux.h
	$(CLANG) $(BPF_CFLAGS) -c pf.bpf.c -o pf.bpf.o

clean:
	rm -f pf.bpf.o vmlinux.h
