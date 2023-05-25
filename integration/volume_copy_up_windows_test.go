//go:build windows

/*
   Copyright The containerd Authors.

   Licensed under the Apache License, Version 2.0 (the "License");
   you may not use this file except in compliance with the License.
   You may obtain a copy of the License at

       http://www.apache.org/licenses/LICENSE-2.0

   Unless required by applicable law or agreed to in writing, software
   distributed under the License is distributed on an "AS IS" BASIS,
   WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
   See the License for the specific language governing permissions and
   limitations under the License.
*/

package integration

import (
	"errors"
	"fmt"
	"os"
	"strings"
	"syscall"

	"golang.org/x/sys/windows"
)

func getOwnership(path string) (string, error) {
	secInfo, err := windows.GetNamedSecurityInfo(
		path, windows.SE_FILE_OBJECT,
		windows.OWNER_SECURITY_INFORMATION|windows.DACL_SECURITY_INFORMATION)

	if err != nil {
		return "", err
	}

	sid, _, err := secInfo.Owner()
	if err != nil {
		return "", err
	}
	return sid.String(), nil
}

func openPath(path string) (windows.Handle, error) {
	u16, err := windows.UTF16PtrFromString(path)
	if err != nil {
		return 0, err
	}
	h, err := windows.CreateFile(
		u16,
		0,
		windows.FILE_SHARE_READ|windows.FILE_SHARE_WRITE|windows.FILE_SHARE_DELETE,
		nil,
		windows.OPEN_EXISTING,
		windows.FILE_FLAG_BACKUP_SEMANTICS, // Needed to open a directory handle.
		0)
	if err != nil {
		return 0, &os.PathError{
			Op:   "CreateFile",
			Path: path,
			Err:  err,
		}
	}
	return h, nil
}

// TODO(gabriel-samfira): expose this function in github.com/Microsoft/go-winio
// We keep needing this in various parts and we're duplicating code.
func getFinalPath(pth string) (string, error) {
	if strings.HasPrefix(pth, `\Device`) {
		pth = `\\.\GLOBALROOT` + pth
	}

	han, err := openPath(pth)
	if err != nil {
		return "", fmt.Errorf("fetching file handle: %w", err)
	}
	defer func() {
		_ = windows.CloseHandle(han)
	}()

	buf := make([]uint16, 100)
	var flags uint32 = 0x0
	for {
		n, err := windows.GetFinalPathNameByHandle(han, &buf[0], uint32(len(buf)), flags)
		if err != nil {
			// if we mounted a volume that does not also have a drive letter assigned, attempting to
			// fetch the VOLUME_NAME_DOS will fail with os.ErrNotExist. Attempt to get the VOLUME_NAME_GUID.
			if errors.Is(err, os.ErrNotExist) && flags != 0x1 {
				flags = 0x1
				continue
			}
			return "", fmt.Errorf("getting final path name: %w", err)
		}
		if n < uint32(len(buf)) {
			break
		}
		buf = make([]uint16, n)
	}
	finalPath := syscall.UTF16ToString(buf)
	// We got VOLUME_NAME_DOS, we need to strip away some leading slashes.
	// Leave unchanged if we ended up requesting VOLUME_NAME_GUID
	if len(finalPath) > 4 && finalPath[:4] == `\\?\` && flags == 0x0 {
		finalPath = finalPath[4:]
		if len(finalPath) > 3 && finalPath[:3] == `UNC` {
			// return path like \\server\share\...
			finalPath = `\` + finalPath[3:]
		}
	}

	return finalPath, nil
}
