//go:build tinygo.cores2 && (rp2040 || rp2350 || esp32s3 || (tinygo.riscv && virt && qemu))

package runtime

const numCPU = 2
