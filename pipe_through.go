package expect

import (
	"io"
	"os"
	"time"
)

// PipeThrough is a Reader with read deadline
// If a timeout is reached, the error is returned
// If the provided io.Reader reads error, the error is also returned
type PipeThrough struct {
	reader *os.File
	errCh  chan error
}

// NewPassthroughPipe returns a new reader for a io.Reader with no read timeout
func NewPipeThrough(reader io.Reader) (*PipeThrough, error) {
	pipeReader, pipeWriter, err := os.Pipe()
	if err != nil {
		return nil, err
	}

	errCh := make(chan error, 1)
	go func() {
		defer close(errCh)
		_, readerErr := io.Copy(pipeWriter, reader)
		if readerErr == nil {
			readerErr = io.EOF
		}

		// Closing the pipeWriter will unblock the pipeReader.Read.
		err = pipeWriter.Close()
		if err != nil {
			panic(err)
			return
		}

		errCh <- readerErr
	}()

	return &PipeThrough{
		reader: pipeReader,
		errCh:  errCh,
	}, nil
}

func (pt *PipeThrough) Read(p []byte) (n int, err error) {
	n, err = pt.reader.Read(p)
	if err != nil {
		if os.IsTimeout(err) {
			return n, err
		}

		// If the pipe is closed, this is the second time calling Read on
		// PassthroughPipe, so just return the error from the os.Pipe io.Reader.
		perr, ok := <-pt.errCh
		if !ok {
			return n, err
		}

		return n, perr
	}

	return n, nil
}

func (pt *PipeThrough) Close() error {
	return pt.reader.Close()
}

func (pt *PipeThrough) SetReadDeadline(t time.Time) error {
	return pt.reader.SetReadDeadline(t)
}
