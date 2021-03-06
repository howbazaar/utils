// Copyright 2014 Canonical Ltd.
// Licensed under the LGPLv3, see LICENCE file for details.

package exec

import (
	"bytes"
	"os"
	"os/exec"
	"runtime"
	"strings"
	"syscall"

	"github.com/juju/errors"

	"github.com/juju/loggo"
)

var logger = loggo.GetLogger("juju.util.exec")

// Parameters for RunCommands.  Commands contains one or more commands to be
// executed using '/bin/bash -s'.  If WorkingDir is set, this is passed
// through to bash.  Similarly if the Environment is specified, this is used
// for executing the command.
type RunParams struct {
	Commands    string
	WorkingDir  string
	Environment []string

	stdout *bytes.Buffer
	stderr *bytes.Buffer
	ps     *exec.Cmd
}

// ExecResponse contains the return code and output generated by executing a
// command.
type ExecResponse struct {
	Code   int
	Stdout []byte
	Stderr []byte
}

// mergeEnvironment takes in a string array representing the desired environment
// and merges it with the current environment. On Windows, clearing the environment,
// or having missing environment variables, may lead to standard go packages not working
// (os.TempDir relies on $env:TEMP), and powershell erroring out
// Currently this function is only used for windows
func mergeEnvironment(env []string) []string {
	if env == nil {
		return nil
	}
	m := make(map[string]string)
	var tmpEnv []string
	for _, val := range os.Environ() {
		varSplit := strings.SplitN(val, "=", 2)
		m[varSplit[0]] = varSplit[1]
	}

	for _, val := range env {
		varSplit := strings.SplitN(val, "=", 2)
		m[varSplit[0]] = varSplit[1]
	}

	for key, val := range m {
		tmpEnv = append(tmpEnv, key+"="+val)
	}

	return tmpEnv
}

// shellAndArgs is a helper function that returns an OS specific
// shell and arguments for that particular shell
func shellAndArgs() (string, []string) {
	var com []string
	switch runtime.GOOS {
	case "windows":
		com = []string{
			"powershell.exe",
			"-noprofile",
			"-noninteractive",
			"-command",
			"try{$input|iex; exit $LastExitCode}catch{Write-Error -Message $Error[0]; exit 1}",
		}
	default:
		com = []string{
			"/bin/bash",
			"-s",
		}
	}
	return com[0], com[1:]
}

// Run sets up the command environment (environment variables, working dir)
// and starts the process. The commands are passed into '/bin/bash -s' through stdin
// on Linux machines and to powershell on Windows machines.
func (r *RunParams) Run() error {
	if runtime.GOOS == "windows" {
		r.Environment = mergeEnvironment(r.Environment)
	}
	shell, args := shellAndArgs()
	r.ps = exec.Command(shell, args...)
	if r.Environment != nil {
		r.ps.Env = r.Environment
	}
	if r.WorkingDir != "" {
		r.ps.Dir = r.WorkingDir
	}
	r.ps.Stdin = bytes.NewBufferString(r.Commands)

	r.stdout = &bytes.Buffer{}
	r.stderr = &bytes.Buffer{}

	r.ps.Stdout = r.stdout
	r.ps.Stderr = r.stderr

	err := r.ps.Start()
	if err != nil {
		return err
	}
	return nil
}

// Process returns the *os.Process instance of the current running process
// This will allow us to kill the process if needed, or get more information
// on the process
func (r *RunParams) Process() *os.Process {
	if r.ps != nil && r.ps.Process != nil {
		return r.ps.Process
	}
	return nil
}

// Wait blocks until the process exits, and returns an ExecResponse type
// containing stdout, stderr and the return code of the process. If a non-zero
// return code is returned, this is collected as the code for the response and
// this does not classify as an error.
func (r *RunParams) Wait() (*ExecResponse, error) {
	var err error
	if r.ps == nil {
		return nil, errors.New("No process has been started yet")
	}
	err = r.ps.Wait()

	result := &ExecResponse{
		Stdout: r.stdout.Bytes(),
		Stderr: r.stderr.Bytes(),
	}

	if ee, ok := err.(*exec.ExitError); ok && err != nil {
		status := ee.ProcessState.Sys().(syscall.WaitStatus)
		if status.Exited() {
			// A non-zero return code isn't considered an error here.
			result.Code = status.ExitStatus()
			err = nil
		}
		logger.Infof("run result: %v", ee)
	}
	return result, err
}

// RunCommands executes the Commands specified in the RunParams using
// powershell on windows, and '/bin/bash -s' on everything else,
// passing the commands through as stdin, and collecting
// stdout and stderr.  If a non-zero return code is returned, this is
// collected as the code for the response and this does not classify as an
// error.
func RunCommands(run RunParams) (*ExecResponse, error) {
	err := run.Run()
	if err != nil {
		return nil, err
	}
	return run.Wait()
}
