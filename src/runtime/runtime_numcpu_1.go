//go:build tinygo.cores1 && (rp2040 || rp2350 || esp32 || esp32s3 || esp32p4 || (tinygo.riscv && virt && qemu))

package runtime

const numCPU = 1
