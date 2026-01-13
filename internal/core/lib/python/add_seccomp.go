package python

import (
	"fmt"
	"os"
	"strconv"
	"strings"
	"syscall"

	"github.com/langgenius/dify-sandbox/internal/core/lib"
	"github.com/langgenius/dify-sandbox/internal/static/python_syscall"
)

//var allow_syscalls = []int{}

// InitSeccomp initializes the security sandbox using seccomp syscall filtering
// and privilege dropping.
//
// Security model:
// 1. NO_NEW_PRIVS - prevents privilege escalation through setuid binaries
// 2. Seccomp - whitelists only required syscalls, kills process on violation
// 3. setgid/setuid - drops to unprivileged user after setup
//
// Note: chroot is intentionally NOT used because:
// - Chroot alone is not a security boundary (can be escaped with root or capabilities)
// - Seccomp provides stronger syscall-level isolation
// - Container-level namespaces (when running in Docker) provide filesystem isolation
// - Chroot would require duplicating Python runtime into the jail
func InitSeccomp(uid int, gid int, enable_network bool) error {
	var err error

	lib.SetNoNewPrivs()

	allowed_syscalls := []int{}
	allowed_not_kill_syscalls := []int{}
	allowed_not_kill_syscalls = append(allowed_not_kill_syscalls, python_syscall.ALLOW_ERROR_SYSCALLS...)

	allowed_syscall := os.Getenv("ALLOWED_SYSCALLS")
	if allowed_syscall != "" {
		nums := strings.Split(allowed_syscall, ",")
		for num := range nums {
			syscall, err := strconv.Atoi(nums[num])
			if err != nil {
				continue
			}
			allowed_syscalls = append(allowed_syscalls, syscall)
		}
	} else {
		allowed_syscalls = append(allowed_syscalls, python_syscall.ALLOW_SYSCALLS...)
		allowed_syscalls = append(allowed_syscalls, python_syscall.ALLOW_FILE_SYSCALLS...)
		if enable_network {
			allowed_syscalls = append(allowed_syscalls, python_syscall.ALLOW_NETWORK_SYSCALLS...)
		}
	}

	err = lib.Seccomp(allowed_syscalls, allowed_not_kill_syscalls)
	if err != nil {
		return err
	}

	// setgid - critical security operation, must succeed
	err = syscall.Setgid(gid)
	if err != nil {
		return fmt.Errorf("failed to setgid to %d: %w", gid, err)
	}

	// setuid - critical security operation, must succeed
	err = syscall.Setuid(uid)
	if err != nil {
		return fmt.Errorf("failed to setuid to %d: %w", uid, err)
	}

	return nil
}
