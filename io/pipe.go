package io

import (
	"errors"
	"io"
	"sync"
)

func Pipe() (*PipeReader, *PipeWriter) {
	p := pipe{
		wrCh: make(chan []byte),
		rdCh: make(chan int),
		done: make(chan struct{}),
	}
	return &PipeReader{p: &p}, &PipeWriter{p: &p}
}

type PipeReader struct {
	p *pipe
}

func (pr *PipeReader) Read(b []byte) (n int, err error) {
	return pr.p.read(b)
}

func (pr *PipeReader) Close() error {
	return pr.CloseWithError(nil)
}

func (pr *PipeReader) CloseWithError(err error) error {
	return pr.p.closeRead(err)
}

type PipeWriter struct {
	p *pipe
}

func (pw *PipeWriter) Write(b []byte) (n int, err error) {
	return pw.p.write(b)
}

func (pw *PipeWriter) Close() error {
	return pw.CloseWithError(nil)
}

func (pw *PipeWriter) CloseWithError(err error) error {
	return pw.p.closeWrite(err)
}

var ErrClosedPipe = errors.New("") // TODO correct error string

type pipe struct {
	wrCh  chan ([]byte)
	rdCh  chan (int)
	wrMux sync.Mutex

	once sync.Once
	done chan (struct{})

	rerr onceError
	werr onceError
}

func (p *pipe) read(b []byte) (n int, err error) {
	select {
	case <-p.done:
		return 0, p.readCloseError()
	default:
	}
	select {
	case bw := <-p.wrCh:
		nr := copy(b, bw)
		p.rdCh <- nr
		return nr, nil
	case <-p.done:
		return 0, p.readCloseError()
	}
}

func (p *pipe) closeRead(err error) error {
	if err == nil {
		err = ErrClosedPipe
	}
	p.rerr.Store(err)
	p.once.Do(func() { close(p.done) })
	return nil
}

func (p *pipe) readCloseError() error {
	werr := p.werr.Load()
	if rerr := p.rerr.Load(); rerr == nil && werr != nil {
		return werr
	}
	return ErrClosedPipe
}

func (p *pipe) write(b []byte) (n int, err error) {
	select {
	case <-p.done:
		return 0, p.writeCloseError()
	default:
		p.wrMux.Lock()
		defer p.wrMux.Unlock()
	}
	for once := true; len(b) > 0 || once; once = false {
		select {
		case p.wrCh <- b:
			nw := <-p.rdCh
			b = b[nw:]
			n += nw
		case <-p.done:
			return n, p.writeCloseError()
		}
	}
	return n, nil
}

func (p *pipe) closeWrite(err error) error {
	if err == nil {
		err = io.EOF
	}
	p.werr.Store(err)
	p.once.Do(func() { close(p.done) })
	return nil
}

func (p *pipe) writeCloseError() error {
	rerr := p.rerr.Load()
	if werr := p.werr.Load(); werr == nil && rerr != nil {
		return rerr
	}
	return ErrClosedPipe
}

// helper type
// 1回だけ書き込めるエラー型
type onceError struct {
	err error
	sync.Mutex
}

func (o *onceError) Load() error {
	o.Lock()
	defer o.Unlock()
	return o.err
}

func (o *onceError) Store(err error) {
	o.Lock()
	defer o.Unlock()
	if o.err != nil {
		return
	}
	o.err = err
}
