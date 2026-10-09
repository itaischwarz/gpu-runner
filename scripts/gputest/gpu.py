"""Tiny GPU probe for run.sh, using the NVIDIA driver (libcuda) directly so the
test needs no PyTorch or other download.

  python3 gpu.py info            # prints: CUDA_VISIBLE_DEVICES, GPU count, GPU name
  python3 gpu.py alloc GB SECS   # holds GB of GPU memory for SECS seconds
"""
import ctypes
import os
import sys
import time

cuda = ctypes.CDLL("libcuda.so.1")


def check(result, what):
    if result != 0:
        sys.exit(f"{what} failed: CUDA error {result}")


check(cuda.cuInit(0), "cuInit")
dev = ctypes.c_int()
check(cuda.cuDeviceGet(ctypes.byref(dev), 0), "cuDeviceGet")

if sys.argv[1] == "info":
    count = ctypes.c_int()
    check(cuda.cuDeviceGetCount(ctypes.byref(count)), "cuDeviceGetCount")
    name = ctypes.create_string_buffer(256)
    check(cuda.cuDeviceGetName(name, 256, dev), "cuDeviceGetName")
    print(os.environ.get("CUDA_VISIBLE_DEVICES"), count.value, name.value.decode())
elif sys.argv[1] == "alloc":
    gb, secs = float(sys.argv[2]), float(sys.argv[3])
    ctx = ctypes.c_void_p()
    check(cuda.cuDevicePrimaryCtxRetain(ctypes.byref(ctx), dev), "cuDevicePrimaryCtxRetain")
    check(cuda.cuCtxSetCurrent(ctx), "cuCtxSetCurrent")
    ptr = ctypes.c_uint64()
    check(cuda.cuMemAlloc_v2(ctypes.byref(ptr), ctypes.c_size_t(int(gb * 1024**3))), "cuMemAlloc")
    time.sleep(secs)
else:
    sys.exit(__doc__)
