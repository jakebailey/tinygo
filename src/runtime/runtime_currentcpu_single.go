//go:build (exec.single || exec.asyncify) && !rp2040 && !rp2350 && !esp32s3 && !(tinygo.riscv && virt && qemu)

package runtime

func currentCPU() uint32 {
	return 0
}
