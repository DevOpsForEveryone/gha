package container

import (
	"fmt"
	"os"
	"runtime"
	"strings"
	"testing"

	log "github.com/sirupsen/logrus"
	"github.com/stretchr/testify/assert"
)

func TestContainerPath(t *testing.T) {
	type containerPathJob struct {
		destinationPath string
		sourcePath      string
		workDir         string
	}

	linuxcontainerext := &LinuxContainerEnvironmentExtensions{}

	if runtime.GOOS == "windows" {
		cwd, err := os.Getwd()
		if err != nil {
			log.Error(err)
		}

		rootDrive := os.Getenv("SystemDrive")
		rootDriveLetter := strings.ReplaceAll(strings.ToLower(rootDrive), `:`, "")
		for _, v := range []containerPathJob{
			{"/mnt/c/Users/gha/go/src/github.com/DevOpsForEveryone/gha", "C:\\Users\\gha\\go\\src\\github.com\\DevOpsForEveryone\\gha\\", ""},
			{"/mnt/f/work/dir", `F:\work\dir`, ""},
			{"/mnt/c/windows/to/unix", "windows\\to\\unix", fmt.Sprintf("%s\\", rootDrive)},
			{fmt.Sprintf("/mnt/%v/gha", rootDriveLetter), "gha", fmt.Sprintf("%s\\", rootDrive)},
		} {
			if v.workDir != "" {
				if err := os.Chdir(v.workDir); err != nil {
					log.Error(err)
					t.Fail()
				}
			}

			assert.Equal(t, v.destinationPath, linuxcontainerext.ToContainerPath(v.sourcePath))
		}

		if err := os.Chdir(cwd); err != nil {
			log.Error(err)
		}
	} else {
		cwd, err := os.Getwd()
		if err != nil {
			log.Error(err)
		}
		for _, v := range []containerPathJob{
			{"/home/gha/go/src/github.com/DevOpsForEveryone/gha", "/home/gha/go/src/github.com/DevOpsForEveryone/gha", ""},
			{"/home/gha", `/home/gha/`, ""},
			{cwd, ".", ""},
		} {
			assert.Equal(t, v.destinationPath, linuxcontainerext.ToContainerPath(v.sourcePath))
		}
	}
}

type typeAssertMockContainer struct {
	Container
	LinuxContainerEnvironmentExtensions
}

// Type assert Container + LinuxContainerEnvironmentExtensions implements ExecutionsEnvironment
var _ ExecutionsEnvironment = &typeAssertMockContainer{}
