/*
Copyright (C) 2022-2023 ApeCloud Co., Ltd

This file is part of KubeBlocks project

This program is free software: you can redistribute it and/or modify
it under the terms of the GNU Affero General Public License as published by
the Free Software Foundation, either version 3 of the License, or
(at your option) any later version.

This program is distributed in the hope that it will be useful
but WITHOUT ANY WARRANTY; without even the implied warranty of
MERCHANTABILITY or FITNESS FOR A PARTICULAR PURPOSE.  See the
GNU Affero General Public License for more details.

You should have received a copy of the GNU Affero General Public License
along with this program.  If not, see <http://www.gnu.org/licenses/>.
*/

package configmanager

import (
	"fmt"
	"os"
)

type PID int32

const (
	InvalidPID PID = 0
)

func sendSignal(pid PID, sig os.Signal) error {
	process, err := os.FindProcess(int(pid))
	if err != nil {
		return err
	}

	logger.Info(fmt.Sprintf("send pid[%d] to signal: %s", pid, sig.String()))
	err = process.Signal(sig)
	if err != nil {
		return err
	}

	return nil
}