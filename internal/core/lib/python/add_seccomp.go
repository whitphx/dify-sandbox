//go:build linux

package python

import (
	"os"
	"strconv"
	"strings"
	"syscall"

	"github.com/langgenius/dify-sandbox/internal/core/lib"
	"github.com/langgenius/dify-sandbox/internal/static/python_syscall"
)

//var allow_syscalls = []int{}

func InitSeccomp(uid int, gid int, enable_network bool) error {
	lib.SetNoNewPrivs()

	allowed_syscalls := []int{}
	allowed_not_kill_syscalls := []int{}

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

		// Filter ALLOW_ERROR_SYSCALLS to exclude syscalls already allowed.
		// This prevents ActErrno rules from overriding ActAllow rules.
		allowedSet := make(map[int]bool)
		for _, sc := range allowed_syscalls {
			allowedSet[sc] = true
		}
		for _, sc := range python_syscall.ALLOW_ERROR_SYSCALLS {
			if !allowedSet[sc] {
				allowed_not_kill_syscalls = append(allowed_not_kill_syscalls, sc)
			}
		}
	}

	err := lib.Seccomp(allowed_syscalls, allowed_not_kill_syscalls)
	if err != nil {
		return err
	}

	// setgid must be called before setuid
	err = syscall.Setgid(gid)
	if err != nil {
		return err
	}

	err = syscall.Setuid(uid)
	if err != nil {
		return err
	}

	return nil
}
