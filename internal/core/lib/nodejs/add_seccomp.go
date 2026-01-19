package nodejs

import (
	"os"
	"strconv"
	"strings"
	"syscall"

	"github.com/langgenius/dify-sandbox/internal/core/lib"
	"github.com/langgenius/dify-sandbox/internal/static/nodejs_syscall"
)

//var allow_syscalls = []int{}

func InitSeccomp(uid int, gid int, enable_network bool) error {
	err := syscall.Chroot(".")
	if err != nil {
		return err
	}
	err = syscall.Chdir("/")
	if err != nil {
		return err
	}

	lib.SetNoNewPrivs()

	allowed_syscalls := []int{}

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
		allowed_syscalls = append(allowed_syscalls, nodejs_syscall.ALLOW_SYSCALLS...)

		if enable_network {
			allowed_syscalls = append(allowed_syscalls, nodejs_syscall.ALLOW_NETWORK_SYSCALLS...)
		}
	}

	// Build a set of allowed syscalls for efficient lookup
	allowedSet := make(map[int]bool)
	for _, sc := range allowed_syscalls {
		allowedSet[sc] = true
	}

	// Filter ALLOW_ERROR_SYSCALLS to exclude any syscalls that are already in allowed_syscalls.
	// This prevents ActErrno rules from overriding ActAllow rules for the same syscall.
	allowed_not_kill_syscalls := []int{}
	for _, sc := range nodejs_syscall.ALLOW_ERROR_SYSCALLS {
		if !allowedSet[sc] {
			allowed_not_kill_syscalls = append(allowed_not_kill_syscalls, sc)
		}
	}

	err = lib.Seccomp(allowed_syscalls, allowed_not_kill_syscalls)
	if err != nil {
		return err
	}

	// setuid
	err = syscall.Setuid(uid)
	if err != nil {
		return err
	}

	// setgid
	err = syscall.Setgid(gid)
	if err != nil {
		return err
	}

	return nil
}
