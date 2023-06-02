package filewriter

// StubWriter is stub writer.
type StubWriter struct {
	size int64
}

// Write implements storagedriver.FileWriter.
func (sw *StubWriter) Write(p []byte) (int, error) {
	sw.size += int64(len(p))
	return len(p), nil
}

// Size implements storagedriver.FileWriter.
func (sw *StubWriter) Size() int64 {
	return sw.size
}

// Close implements storagedriver.FileWriter.
func (sw *StubWriter) Close() error {
	return nil
}

// Cancel implements storagedriver.FileWriter.
func (sw *StubWriter) Cancel() error {
	return nil
}

// Commit implements storagedriver.FileWriter.
func (sw *StubWriter) Commit() error {
	return nil
}
