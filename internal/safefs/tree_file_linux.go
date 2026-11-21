//go:build linux

package safefs

import (
	"errors"
	"os"

	"golang.org/x/sys/unix"
)

// treeRead keeps all reads positional, including the one-byte growth probe.
func treeRead(s *stageState, parent edge, name string, limit int64, collect, synchronize bool) (data []byte, stamp fileStamp, digest [32]byte, err error) {
	var pin *pinnedFile
	var proc *procAccess
	var reader *os.File
	defer func() {
		var closed error
		if reader != nil {
			closed = reader.Close()
		}
		if errors.Join(closed, proc.close(), pin.close()) != nil {
			err = errDirectory
		}
		if err != nil {
			data = nil
			stamp = fileStamp{}
			digest = [32]byte{}
		}
	}()
	pin, err = pinFile(parent, name, limit)
	if err != nil {
		return nil, fileStamp{}, digest, errDirectory
	}
	if !safeTreeEntry(pin.before, parent.id, s.owner) {
		return nil, fileStamp{}, digest, errDirectory
	}
	proc, err = openProcAccess()
	if err != nil {
		return nil, fileStamp{}, digest, errDirectory
	}
	reader, err = proc.reopen(pin)
	if err != nil || pin.validate() != nil {
		return nil, fileStamp{}, digest, errDirectory
	}
	data, digest, err = readFileContents(reader, pin.before.identity.Size, collect, func() error {
		after, failure := observeFile(int(reader.Fd()), "")
		if failure != nil || after != pin.before || pin.validate() != nil || proc.validate() != nil {
			return errDirectory
		}
		return nil
	}, s.readCharge)
	if err != nil {
		return nil, fileStamp{}, digest, errDirectory
	}
	if synchronize && unix.Fsync(int(reader.Fd())) != nil {
		return nil, fileStamp{}, digest, errDirectory
	}
	after, failure := observeFile(int(reader.Fd()), "")
	if failure != nil || after != pin.before || pin.validate() != nil || proc.validate() != nil {
		return nil, fileStamp{}, digest, errDirectory
	}
	return data, pin.before, digest, nil
}

func (d *Dir) treeReadFile(name string, limit int64) (data []byte, id Identity, err error) {
	ds := d.state
	if len(name) > 255 {
		return nil, Identity{}, errDirectory
	}
	// Reserve the caller's requested upper bound before any metadata I/O.
	if limit > treeMaxFile {
		return nil, Identity{}, errDirectory
	}
	op, err := ds.gate.begin(8, 32<<10, uint64(limit)+65536)
	if err != nil {
		return nil, Identity{}, errDirectory
	}
	failed := false
	defer func() {
		if !op.finish(failed) {
			data = nil
			id = Identity{}
			err = errDirectory
		}
	}()
	ds.mu.Lock()
	defer ds.mu.Unlock()
	if ds.closed {
		return nil, Identity{}, errDirectory
	}
	if _, ok := stageRelative(ds, name); !ok {
		return nil, Identity{}, errDirectory
	}
	failed = true
	if publicationChain(ds) != nil {
		return nil, Identity{}, errDirectory
	}
	data, stamp, _, err := treeRead(ds.gate, ds.chain[len(ds.chain)-1], name, limit, true, false)
	if err != nil || publicationChain(ds) != nil {
		return nil, Identity{}, errDirectory
	}
	failed = false
	return data, stamp.identity, nil
}

func (d *Dir) treePublishFile(name string, data []byte, mode uint32) (id Identity, err error) {
	ds := d.state
	if len(name) > 255 || len(data) > treeMaxFile {
		return Identity{}, errDirectory
	}
	op, err := ds.gate.begin(3, 32<<10, 65536)
	if err != nil {
		return Identity{}, errDirectory
	}
	failed := false
	defer func() {
		if !op.finish(failed) {
			id = Identity{}
			if err == nil {
				err = errDirectory
			}
		}
	}()
	ds.mu.Lock()
	defer ds.mu.Unlock()
	if ds.closed {
		return Identity{}, errDirectory
	}
	pathBytes, ok := stageRelative(ds, name)
	if !ok {
		return Identity{}, errDirectory
	}
	if !ds.gate.addContent(pathBytes, uint64(len(data))) {
		return Identity{}, errDirectory
	}
	failed = true
	id, err = publishFileLocked(ds, name, data, mode)
	if err == nil {
		failed = false
	}
	return id, err
}
