package service

import (
	"bytes"
	"context"
	"errors"
	"io"
	"sort"
	"strings"
	"sync"
	"time"
	"unicode"

	"github.com/Cail-Gainey/MineOps/internal/global/apperror"
	"github.com/Cail-Gainey/MineOps/internal/model"
	"golang.org/x/crypto/ssh"
)

// RemoteCommand 承载结构化的 POSIX 命令,不预先拼接用户 shell 文本。
type RemoteCommand struct {
	Executable       string
	Arguments        []string
	Environment      map[string]string
	WorkingDirectory string
	Input            []byte
	InputReader      io.Reader
	OutputWriter     io.Writer
	Timeout          time.Duration
	MaximumOutput    int
	OnOutput         func(RemoteCommandOutput)
}

// RemoteCommandOutput 是运行中命令的一个有界、尽力而为的 stdout/stderr 批次。
type RemoteCommandOutput struct {
	Stream string
	Data   string
}

// RemoteCommandResult 承载有界的 stdout/stderr、退出状态与耗时。
type RemoteCommandResult struct {
	Stdout    string
	Stderr    string
	ExitCode  int
	Duration  time.Duration
	Truncated bool
}

// RunCommand 打开一条短生命周期的 SSH 通道并执行一条结构化 POSIX 命令。
func (c *SSHClient) RunCommand(ctx context.Context, command RemoteCommand) (RemoteCommandResult, error) {
	if c == nil || c.client == nil {
		return RemoteCommandResult{}, apperror.New(apperror.CodeSSHConnectionFailed, "SSH Client 不可用")
	}
	commandLine, err := command.posixShellLine()
	if err != nil {
		return RemoteCommandResult{}, err
	}
	if len(command.Input) > 0 && command.InputReader != nil {
		return RemoteCommandResult{}, apperror.New(apperror.CodeValidationConflict, "远程命令不能同时使用字节输入和流式输入")
	}
	maximumOutput := command.MaximumOutput
	if maximumOutput <= 0 {
		maximumOutput = 1024 * 1024
	}
	var outputContext context.Context
	var cancelOutput context.CancelFunc
	var outputChannel chan RemoteCommandOutput
	var outputWait sync.WaitGroup
	if command.OnOutput != nil {
		outputContext, cancelOutput = context.WithCancel(context.Background())
		outputChannel = make(chan RemoteCommandOutput, 64)
		outputWait.Add(1)
		go func() {
			defer outputWait.Done()
			for {
				select {
				case output := <-outputChannel:
					command.OnOutput(output)
				case <-outputContext.Done():
					for {
						select {
						case output := <-outputChannel:
							command.OnOutput(output)
						default:
							return
						}
					}
				}
			}
		}()
		defer func() {
			cancelOutput()
			outputWait.Wait()
		}()
	}
	stdout := newBoundedCommandBuffer(maximumOutput, "stdout", outputContext, outputChannel)
	stderr := newBoundedCommandBuffer(maximumOutput, "stderr", outputContext, outputChannel)
	session, err := c.client.NewSession()
	if err != nil {
		return RemoteCommandResult{}, apperror.Wrap(apperror.CodeSSHConnectionFailed, "创建 SSH 命令通道失败", err)
	}
	defer func() { _ = session.Close() }()
	if command.InputReader != nil {
		session.Stdin = command.InputReader
	} else if len(command.Input) > 0 {
		session.Stdin = bytes.NewReader(command.Input)
	}
	if command.OutputWriter != nil {
		session.Stdout = io.MultiWriter(command.OutputWriter, stdout)
	} else {
		session.Stdout = stdout
	}
	session.Stderr = stderr
	startedAt := time.Now()
	commandResult := make(chan error, 1)
	go func() { commandResult <- session.Run(commandLine) }()
	commandCtx := ctx
	var cancel context.CancelFunc
	if command.Timeout > 0 {
		commandCtx, cancel = context.WithTimeout(ctx, command.Timeout)
		defer cancel()
	}
	select {
	case <-commandCtx.Done():
		_ = session.Close()
		select {
		case <-commandResult:
		case <-time.After(time.Second):
		}
		result := RemoteCommandResult{
			Stdout: stdout.String(), Stderr: stderr.String(), Duration: time.Since(startedAt),
			Truncated: stdout.Truncated() || stderr.Truncated(),
		}
		return result, apperror.Wrap(apperror.CodeProcessCancelled, "远程命令已取消或超时", commandCtx.Err()).WithDetails(map[string]any{
			"executable": command.Executable, "stdout": result.Stdout, "stderr": result.Stderr,
		})
	case runError := <-commandResult:
		result := RemoteCommandResult{
			Stdout: stdout.String(), Stderr: stderr.String(), Duration: time.Since(startedAt),
			Truncated: stdout.Truncated() || stderr.Truncated(),
		}
		if runError == nil {
			return result, nil
		}
		var exitError *ssh.ExitError
		if errors.As(runError, &exitError) {
			result.ExitCode = exitError.ExitStatus()
			return result, apperror.Wrap(apperror.CodeProcessExitFailed, "远程命令退出状态非零", runError).WithDetails(map[string]any{
				"executable": command.Executable, "exitCode": result.ExitCode, "stderr": result.Stderr,
			})
		}
		return result, apperror.Wrap(apperror.CodeProcessExitFailed, "远程命令执行失败", runError)
	}
}

func (c RemoteCommand) posixShellLine() (string, error) {
	if strings.TrimSpace(c.Executable) == "" || strings.ContainsRune(c.Executable, '\x00') {
		return "", apperror.New(apperror.CodeValidationRequired, "远程命令 Executable 不能为空")
	}
	parts := make([]string, 0, len(c.Environment)+len(c.Arguments)+4)
	if c.WorkingDirectory != "" {
		if strings.ContainsRune(c.WorkingDirectory, '\x00') {
			return "", apperror.New(apperror.CodeValidationInvalidArgument, "远程工作目录无效")
		}
		parts = append(parts, "cd -- "+quotePOSIX(c.WorkingDirectory)+" &&")
	}
	keys := make([]string, 0, len(c.Environment))
	for key := range c.Environment {
		if !validEnvironmentName(key) {
			return "", apperror.New(apperror.CodeValidationInvalidArgument, "远程环境变量名称无效")
		}
		keys = append(keys, key)
	}
	sort.Strings(keys)
	for _, key := range keys {
		parts = append(parts, key+"="+quotePOSIX(c.Environment[key]))
	}
	parts = append(parts, "exec", quotePOSIX(c.Executable))
	for _, argument := range c.Arguments {
		if strings.ContainsRune(argument, '\x00') {
			return "", apperror.New(apperror.CodeValidationInvalidArgument, "远程命令参数包含 NUL 字符")
		}
		parts = append(parts, quotePOSIX(argument))
	}
	return strings.Join(parts, " "), nil
}

func quotePOSIX(value string) string {
	return "'" + strings.ReplaceAll(value, "'", "'\"'\"'") + "'"
}

func validEnvironmentName(value string) bool {
	if value == "" || !unicode.IsLetter(rune(value[0])) && value[0] != '_' {
		return false
	}
	for _, character := range value[1:] {
		if !unicode.IsLetter(character) && !unicode.IsDigit(character) && character != '_' {
			return false
		}
	}
	return true
}

type boundedCommandBuffer struct {
	mu        sync.Mutex
	buffer    bytes.Buffer
	maximum   int
	truncated bool
	stream    string
	outputCtx context.Context
	output    chan<- RemoteCommandOutput
}

func newBoundedCommandBuffer(maximum int, stream string, outputCtx context.Context, output chan<- RemoteCommandOutput) *boundedCommandBuffer {
	return &boundedCommandBuffer{maximum: maximum, stream: stream, outputCtx: outputCtx, output: output}
}

// Write 写入命令输出,超过上限后丢弃多余内容并标记截断。
func (b *boundedCommandBuffer) Write(value []byte) (int, error) {
	b.mu.Lock()
	remaining := b.maximum - b.buffer.Len()
	if remaining > 0 {
		written := len(value)
		if written > remaining {
			written = remaining
		}
		_, _ = b.buffer.Write(value[:written])
	}
	if len(value) > remaining {
		b.truncated = true
	}
	b.mu.Unlock()
	if b.output != nil && len(value) > 0 {
		output := RemoteCommandOutput{Stream: b.stream, Data: string(append([]byte(nil), value...))}
		select {
		case b.output <- output:
		case <-b.outputCtx.Done():
		default:
		}
	}
	return len(value), nil
}

// String 返回已保留的命令输出。
func (b *boundedCommandBuffer) String() string {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.buffer.String()
}

// Truncated 返回输出是否因超过上限而被截断。
func (b *boundedCommandBuffer) Truncated() bool {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.truncated
}

// RunCommand 用一个 SSH Session 建连、执行结构化命令,随后关闭传输层。
func (f *SSHClientFactory) RunCommand(ctx context.Context, session *model.SSHSession, settings model.SSHSettings, command RemoteCommand) (RemoteCommandResult, error) {
	client, err := f.Connect(ctx, session, settings)
	if err != nil {
		return RemoteCommandResult{}, err
	}
	defer func() { _ = client.Close() }()
	return client.RunCommand(ctx, command)
}
