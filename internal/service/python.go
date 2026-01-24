package service

import (
	"context"
	"fmt"
	"io"
	"os"
	"strings"
	"time"

	"github.com/langgenius/dify-sandbox/internal/core/runner/python"
	runner_types "github.com/langgenius/dify-sandbox/internal/core/runner/types"
	"github.com/langgenius/dify-sandbox/internal/static"
	"github.com/langgenius/dify-sandbox/internal/storage"
	"github.com/langgenius/dify-sandbox/internal/types"
)

type RunCodeResponse struct {
	Stderr string            `json:"error"`
	Stdout string            `json:"stdout"`
	Files  map[string]string `json:"files"`
}

// inputFiles is map[filename]file_id
func RunPython3Code(ctx context.Context, code string, preload string, enableNetwork bool, inputFiles map[string]string, fetchFiles []string) *types.DifySandboxResponse {
	// Reconstruct options
	options := &runner_types.RunnerOptions{
		EnableNetwork: enableNetwork,
		FetchFiles:    fetchFiles,
		InputFiles:    make(map[string]io.Reader),
	}

	if err := checkOptions(options); err != nil {
		return types.ErrorResponse(-400, err.Error())
	}

	// Prepare Input Files
	store := storage.GetStorage()
	var readersToClose []io.ReadCloser
	defer func() {
		for _, r := range readersToClose {
			r.Close()
		}
	}()

	for filename, fileId := range inputFiles {
		reader, err := store.Get(fileId)
		if err != nil {
			return types.ErrorResponse(-400, fmt.Sprintf("failed to get input file %s: %v", filename, err))
		}
		options.InputFiles[filename] = reader
		readersToClose = append(readersToClose, reader)
	}

	// Prepare Output Handler
	options.OutputHandler = func(filename, localPath string) (string, error) {
		f, err := os.Open(localPath)
		if err != nil {
			return "", err
		}
		defer f.Close()
		// Upload to storage
		return store.Put(f, filename)
	}

	if !static.GetDifySandboxGlobalConfigurations().EnablePreload {
		preload = ""
	}

	timeout := time.Duration(
		static.GetDifySandboxGlobalConfigurations().WorkerTimeout * int(time.Second),
	)

	runner := python.PythonRunner{}
	stdout, stderr, done, filesChan, err := runner.Run(ctx,
		code, timeout, nil, preload, options,
	)
	if err != nil {
		return types.ErrorResponse(-500, err.Error())
	}

	var stdoutStr strings.Builder
	var stderrStr strings.Builder
	var files map[string]string

	defer close(done)

	for {
		select {
		case <-done:
			// Drain any remaining buffered output to avoid races
		drain:
			for {
				select {
				case out := <-stdout:
					stdoutStr.Write(out)
				case errOut := <-stderr:
					stderrStr.Write(errOut)
				case f := <-filesChan:
					files = f
				default:
					break drain
				}
			}
			// Close channels after draining all data
			close(stdout)
			close(stderr)
			return types.SuccessResponse(&RunCodeResponse{
				Stdout: stdoutStr.String(),
				Stderr: stderrStr.String(),
				Files:  files,
			})
		case out := <-stdout:
			stdoutStr.Write(out)
		case errOut := <-stderr:
			stderrStr.Write(errOut)
		case f := <-filesChan:
			files = f
			filesChan = nil // Stop listening to avoid busy loop on closed channel
		}
	}
}

type ListDependenciesResponse struct {
	Dependencies []runner_types.Dependency `json:"dependencies"`
}

func ListPython3Dependencies() *types.DifySandboxResponse {
	return types.SuccessResponse(&ListDependenciesResponse{
		Dependencies: python.ListDependencies(),
	})
}

type RefreshDependenciesResponse struct {
	Dependencies []runner_types.Dependency `json:"dependencies"`
}

func RefreshPython3Dependencies() *types.DifySandboxResponse {
	return types.SuccessResponse(&RefreshDependenciesResponse{
		Dependencies: python.RefreshDependencies(),
	})
}

type UpdateDependenciesResponse struct{}

func UpdateDependencies() *types.DifySandboxResponse {
	err := python.PreparePythonDependenciesEnv()
	if err != nil {
		return types.ErrorResponse(-500, err.Error())
	}

	return types.SuccessResponse(&UpdateDependenciesResponse{})
}
