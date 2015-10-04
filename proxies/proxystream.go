package proxies

import (
	"bufio"
	"bytes"
	"crypto/tls"
	"crypto/x509"
	"errors"
	"io"
	"io/ioutil"
	"log"
	"net"
	"net/http"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/influx6/flux"
)

func checkError(err error, msg string) {
	if err != nil {
		log.Printf("%s | Errors: (%s)", msg, err.Error())
		return
	}
	log.Printf("%s | NoErrors", msg)
}

//Close returns nil
func (n *Nopwriter) Close() error {
	return nil
}

//NopWriter returns a new nopwriter instance
func NopWriter(w io.Writer) *Nopwriter {
	return &Nopwriter{w}
}

//NewProxyStream returns a new proxy streamer
func NewProxyStream(proc ConnectionProcess, he ErrorHandler) (p *ProxyStream) {

	p = &ProxyStream{
		closer:       make(Notifier, 1),
		errors:       make(NotifierError, 1),
		work:         make(WorkNotifier),
		endWorker:    make(chan int64),
		addWorker:    make(chan int64),
		do:           new(sync.Once),
		halter:       new(sync.Once),
		waiter:       new(sync.WaitGroup),
		resumer:      nil,
		handleErrors: he,
		processor:    proc,
		workers:      0,
	}

	go p.management()
	p.AddWorker()

	return
}

//NewInsight returns a new ConnInsight for use
func NewInsight(src, dest Conn) *ConnInsight {
	return &ConnInsight{
		src:    src,
		dest:   dest,
		in:     flux.NewIdentityStream(),
		out:    flux.NewIdentityStream(),
		closed: flux.NewIdentityStream(),
		Meta:   make(map[string]string),
	}
}

//NewBaseConn returns a new baseconn
func NewBaseConn() *BaseConn {
	return &BaseConn{
		closer: make(Notifier),
		doend:  new(sync.Once),
	}
}

//NewStreamConn returns a new streamConn
func NewStreamConn(c net.Conn) *StreamConn {
	return &StreamConn{BaseConn: NewBaseConn(), src: c}
}

//Src returns the underline src as a interface(requires type-assert for real type)
func (p *StreamConn) Src() interface{} {
	return p.src
}

//Dest returns the dest connection of this req
func (p *ConnInsight) Dest() Conn {
	return p.dest
}

//Src returns the src connection of this req
func (p *ConnInsight) Src() Conn {
	return p.src
}

//Closed returns the connection closed notification stream
func (p *ConnInsight) Closed() flux.StackStreamers {
	return p.closed
}

//Out returns the connection outgoing data
func (p *ConnInsight) Out() flux.StackStreamers {
	return p.out
}

//In returns the connection incoming data
func (p *ConnInsight) In() flux.StackStreamers {
	return p.in
}

//Close closes the connection
func (p *ConnInsight) Close() error {
	ce := p.src.Close()
	cd := p.src.Close()

	_ = p.in.Close()
	_ = p.out.Close()

	p.closed.Emit(p)

	if ce != nil {
		return ce
	}

	return cd
}

//Close closes the connection
func (p *StreamConn) Close() (err error) {
	p.doend.Do(func() {
		close(p.closer)
		err = p.src.Close()
	})
	return
}

//Reader returns the reader for the conn
func (p *StreamConn) Reader() io.ReadCloser {
	return io.ReadCloser(p.src)
}

//Writer returns the writer for the conn
func (p *StreamConn) Writer() io.WriteCloser {
	return io.WriteCloser(p.src)
}

//CloseNotify returns the internal close channel
func (p *BaseConn) CloseNotify() Notifier {
	return p.closer
}

//GetProcessor returns the internal processor function used by the streamer
func (p *ProxyStream) GetProcessor() ConnectionProcess {
	return p.processor
}

//Closed closes the connection
func (p *ProxyStream) Closed() bool {
	return p.do == nil
}

//management initiates the processing of streams
func (p *ProxyStream) management() {
	func() {
	mloop:
		for {
			select {
			case id := <-p.addWorker:
				p.handleStreams(id)
			case <-p.closer:
				break mloop
			case err := <-p.errors:
				if p.handleErrors != nil {
					p.handleErrors(err)
				}
			}
		}
	}()
}

//RemoveWorker reduces the proxystream workers
func (p *ProxyStream) RemoveWorker() {
	cur := atomic.LoadInt64(&p.workers)
	{
		p.endWorker <- cur
	}
	atomic.AddInt64(&p.workers, -1)
}

//AddWorker increase the proxystream workers
func (p *ProxyStream) AddWorker() {
	cur := atomic.AddInt64(&p.workers, 1)
	{
		p.addWorker <- cur
	}
}

//Wait allows servers to check if the director is blocking
func (p *ProxyStream) Wait() {
	p.waiter.Wait()
}

//Pause adds to the director WaitGroup which blocks all
//servers that use it for sync
func (p *ProxyStream) Pause() {
	p.halter.Do(func() {
		p.waiter.Add(1)
		p.resumer = new(sync.Once)
	})
}

//Resume completes the waiter for release the waiter
func (p *ProxyStream) Resume() {
	if p.resumer == nil {
		return
	}

	defer func() {
		p.resumer = nil
	}()

	p.resumer.Do(func() {
		p.waiter.Done()
		p.halter = new(sync.Once)
	})
}

//handleStreams initiates the processing of streams
func (p *ProxyStream) handleStreams(id int64) {
	go func() {
	hloop:
		for {
			select {
			case k := <-p.endWorker:
				if id == k {
					break hloop
				}
			case wk := <-p.work:
				flux.GoDefer("ProxyStreamProcessor", func() {
					p.Wait()
					p.processor(wk, p.errors)
				})

			case <-p.closer:
				break hloop
			}
		}
	}()
}

//Stream handling the stacking of proxy work
func (p *ProxyStream) Stream(src, dest Conn) (*ConnInsight, error) {
	if p.do == nil {
		return nil, ErrBadStreamer
	}

	if src == nil || dest == nil {
		return nil, ErrorNoConnection
	}

	in := NewInsight(src, dest)
	go func() { p.work <- in }()
	return in, nil
}

//Close stops the streaming
func (p *ProxyStream) Close() error {
	defer func() { p.do = nil }()
	p.do.Do(func() {
		close(p.closer)
	})
	return nil
}

//DoBroker provides the internal copy operation
func DoBroker(dest io.WriteCloser, src io.ReadCloser, end io.Closer, err NotifierError) {
	_, ex := io.Copy(dest, src)

	if ex != nil {
		go func() { err <- ex }()
	}

	end.Close()
}

//Close closes the connection
func (p *ReqResConn) Close() (err error) {
	p.doend.Do(func() {})
	return
}

//Src returns the request as the src for the conn
func (p *ReqResConn) Src() interface{} {
	return p.Req
}

//Reader returns the reader for the conn
func (p *ReqResConn) Reader() io.ReadCloser {
	return p.Req.Body
}

//Writer returns the writer for the conn
func (p *ReqResConn) Writer() io.WriteCloser {
	return NopWriter(p.Res)
}

//NewReqRes returns a new ReqResConn
func NewReqRes(res http.ResponseWriter, req *http.Request) *ReqResConn {
	return &ReqResConn{
		BaseConn: NewBaseConn(),
		Req:      req,
		Res:      res,
		Addr:     req.URL.String(),
	}
}

//NewTLSFromConn returns a new StreamConn with the created tls net.Conn
func NewTLSFromConn(con net.Conn, addr string) (*StreamConn, error) {
	if addr == "" {
		return nil, ErrorBadRequestType
	}

	var conf *tls.Config

	tl, ok := con.(*tls.Conn)

	if !ok {
		return nil, ErrBadConn
	}

	err := tl.Handshake()

	if err != nil {
		return nil, err
	}

	state := tl.ConnectionState()

	pool := x509.NewCertPool()

	for _, v := range state.PeerCertificates {
		pool.AddCert(v)
	}

	conf = &tls.Config{
		RootCAs: pool,
	}

	ns, err := tls.Dial("tcp", addr, conf)

	if err != nil {
		log.Printf("Unable to establish tls connection %s", addr)
		return nil, err
	}

	return NewStreamConn(ns), nil
}

//NewTLSConn returns a new StreamConn with the created tls net.Conn
func NewTLSConn(addr string, conf *tls.Config) (*StreamConn, error) {

	if addr == "" {
		return nil, ErrorBadRequestType
	}

	if conf == nil {
		conf = &tls.Config{}
	}

	ns, err := tls.Dial("tcp", addr, conf)

	if err != nil {
		log.Printf("Unable to establish tls connection %s", addr)
		return nil, err
	}

	return NewStreamConn(ns), nil
}

//NewTypeConn returns a new StreamConn with the created net.Conn
func NewTypeConn(addr, ts string, conf *tls.Config) (*StreamConn, error) {
	if addr == "" {
		return nil, ErrorBadRequestType
	}

	var ns net.Conn
	var err error

	if conf == nil {
		ns, err = net.Dial(ts, addr)
	} else {
		ns, err = tls.Dial(ts, addr, conf)
	}

	if err != nil {
		log.Printf("Unable to establish net connection %s", addr)
		return nil, err
	}

	return NewStreamConn(ns), nil
}

//NewNetConn returns a new StreamConn with the created http net.Conn
func NewNetConn(addr string) (*StreamConn, error) {
	if addr == "" {
		return nil, ErrorBadRequestType
	}

	ns, err := net.Dial("tcp", addr)

	if err != nil {
		log.Printf("Unable to establish net connection %s", addr)
		return nil, err
	}

	return NewStreamConn(ns), nil
}

//TargetReqRes returns a new ReqResConn
func TargetReqRes(target string) (*ReqResConn, error) {

	if target == "" {
		return nil, ErrorBadRequestType
	}

	req, err := http.NewRequest("", target, nil)

	if err != nil {
		return nil, err
	}

	return &ReqResConn{
		BaseConn: NewBaseConn(),
		Req:      req,
		Addr:     target,
	}, nil
}

//handleHTTPConns pairing of two net.Conns of http origins
func handleHTTPConns(c *ConnInsight, serr NotifierError) {

	defer c.Close()

	src := c.Src()
	dest := c.Dest()

	rsrc, ok := src.Src().(net.Conn)

	if !ok {
		go func() {
			serr <- ErrBadConn
		}()
		return
	}

	rdest, ok := dest.Src().(net.Conn)

	if !ok {
		go func() {
			serr <- ErrBadConn
		}()
		return
	}

	reader := bufio.NewReader(rsrc)
	req, err := http.ReadRequest(reader)

	if err != nil {
		checkError(err, "Unable to create/generate src connection")
		go func() {
			serr <- err
		}()
		return
	}

	if req == nil {
		log.Printf("Proxy: Received No Request (%+s)!", err)
		if err == nil {
			err = ErrNoRequest
		}
		go func() {
			serr <- err
		}()
		return
	}

	destwriter := NopWriter(io.MultiWriter(rdest, c.In()))
	srcwriter := NopWriter(io.MultiWriter(rsrc, c.Out()))

	req.Write(destwriter)

	resread := bufio.NewReader(rdest)
	res, err := http.ReadResponse(resread, req)

	if err != nil {
		log.Printf("Proxy: ReadResponse failed (%+s)!", err)
		go func() {
			serr <- err
		}()
		return
	}

	if res == nil {
		log.Printf("Proxy: NoResponse received!")
		if err == nil {
			err = ErrNoResponse
		}
		go func() {
			serr <- err
		}()
		return
	}

	res.Write(srcwriter)
}

//handleHTTPReqRes handles pairing between two http.Request and http.ResponseWriter/http.Response
func handleHTTPReqRes(c *ConnInsight, serr NotifierError) {

	defer c.Close()

	src := c.Src()
	dest := c.Dest()

	if src.Src() == nil || dest.Src() == nil {
		go func() {
			serr <- ErrBadConn
		}()
		return
	}

	rsrc, ok := src.(*ReqResConn)

	if !ok {
		go func() {
			serr <- ErrBadConn
		}()
		return
	}

	rdest, ok := dest.(*ReqResConn)

	if !ok {
		go func() {
			serr <- ErrBadConn
		}()
		return
	}

	sreq, sres := rsrc.Req, rsrc.Res
	dreq := rdest.Req

	//set the method
	dreq.Method = sreq.Method

	//copy headers over
	for k, v := range sreq.Header {
		dreq.Header.Add(k, strings.Join(v, ","))
	}

	//remove unwanted headers
	for _, v := range hopHeaders {
		dreq.Header.Del(v)
	}

	ip, _, err := net.SplitHostPort(sreq.RemoteAddr)

	if err == nil {
		//add us to the proxy list or makeone
		hops, ok := sreq.Header["X-Forwarded-For"]
		if ok {
			ip = strings.Join(hops, ",") + "," + ip
		}
		dreq.Header.Set("X-Forwarded-For", ip)
	}

	//copy the body over
	var buf bytes.Buffer

	if sreq.Body != nil {
		io.Copy(&buf, sreq.Body)
	}

	if buf.Len() > 0 {
		dreq.Body = ioutil.NopCloser(&buf)
		dreq.ContentLength = int64(buf.Len())

		var bu []byte
		bu = append(bu, buf.Bytes()...)
		log.Printf("Copied ByteContent of Len %d", len(bu))
		c.In().Emit(bu)
	}

	res, err := httpclient.Do(dreq)

	if err != nil {
		log.Printf("Proxy: ReadResponse failed (%+s)!", err)
		go func() {
			serr <- err
		}()
		return
	}

	if res == nil {
		log.Printf("Proxy: NoResponse received!")
		if err == nil {
			err = ErrNoResponse
		}
		go func() {
			serr <- err
		}()
		return
	}

	// rdest.Res = res

	for k, v := range res.Header {
		sres.Header().Add(k, strings.Join(v, ","))
	}

	srcwriter := io.MultiWriter(sres, c.Out())
	io.Copy(srcwriter, res.Body)
}

//handleHTTP2Net handles pairing between a net.Conn and a addr(with http.Request and http.Response)
func handleHTTP2Net(c *ConnInsight, serr NotifierError) {

	defer c.Close()

	src := c.Src()
	dest := c.Dest()

	if src.Src() == nil || dest.Src() == nil {
		go func() {
			serr <- ErrBadConn
		}()
		return
	}

	dcon, _ := dest.(*ReqResConn)

	scon, ok := src.Src().(net.Conn)

	if !ok {
		go func() {
			serr <- ErrBadConn
		}()
		return
	}

	reader := bufio.NewReader(scon)
	sreq, err := http.ReadRequest(reader)

	if err != nil {
		checkError(err, "Unable to create/generate src connection")
		go func() {
			serr <- err
		}()
		return
	}

	if sreq == nil {
		log.Printf("Proxy: Received No Request (%+s)!", err)
		if err == nil {
			err = ErrNoRequest
		}
		go func() {
			serr <- err
		}()
		return
	}

	dreq := dcon.Req
	//set the method
	dreq.Method = sreq.Method

	//copy headers over
	for k, v := range sreq.Header {
		dreq.Header.Add(k, strings.Join(v, ","))
	}

	//remove unwanted headers
	for _, v := range hopHeaders {
		dreq.Header.Del(v)
	}

	ip, _, err := net.SplitHostPort(sreq.RemoteAddr)

	if err == nil {
		//add us to the proxy list or makeone
		hops, ok := sreq.Header["X-Forwarded-For"]
		if ok {
			ip = strings.Join(hops, ",") + "," + ip
		}
		dreq.Header.Set("X-Forwarded-For", ip)
	}

	//copy the body over
	var buf bytes.Buffer

	if sreq.Body != nil {
		io.Copy(&buf, sreq.Body)
	}

	if buf.Len() > 0 {
		dreq.Body = ioutil.NopCloser(&buf)
		dreq.ContentLength = int64(buf.Len())

		var bu []byte
		bu = append(bu, buf.Bytes()...)
		log.Printf("Copied ByteContent of Len %d", len(bu))
		c.In().Emit(bu)
	}

	res, err := httpclient.Do(dreq)

	if err != nil {
		log.Printf("Proxy: ReadResponse failed (%+s)!", err)
		go func() {
			serr <- err
		}()
		return
	}

	if res == nil {
		log.Printf("Proxy: NoResponse received!")
		if err == nil {
			err = ErrNoResponse
		}
		go func() {
			serr <- err
		}()
		return
	}

	// dcon.Res = res

	srcwriter := NopWriter(io.MultiWriter(scon, c.Out()))

	res.Write(srcwriter)
}

//handleNet2HTTP handles when a http Request and Response source are paired with a net.Conn destination
func handleNet2HTTP(c *ConnInsight, serr NotifierError) {

	//always close the conn insight
	defer c.Close()

	src := c.Src()
	dest := c.Dest()

	if src.Src() == nil || dest.Src() == nil {
		go func() {
			serr <- ErrBadConn
		}()
		return
	}

	dcon, ok := dest.(*StreamConn)

	if !ok {
		go func() {
			serr <- ErrBadConn
		}()
		return
	}

	scon, ok := src.(*ReqResConn)

	if !ok {
		go func() {
			serr <- ErrBadConn
		}()
		return
	}

	destcon, ok := dcon.Src().(net.Conn)

	if !ok {
		go func() {
			serr <- ErrBadConn
		}()
		return
	}

	req, res := scon.Req, scon.Res

	if req == nil || res == nil {
		go func() {
			serr <- ErrorBadHTTPPacketType
		}()
		return
	}

	destw := io.MultiWriter(destcon, c.In())

	req.Write(destw)

	resread := bufio.NewReader(destcon)
	dres, err := http.ReadResponse(resread, req)

	if err != nil {
		log.Printf("Proxy: ReadResponse failed (%+s)!", err)
		go func() {
			serr <- err
		}()
		return
	}

	if dres == nil {
		log.Printf("Proxy: NoResponse received!")
		if err == nil {
			err = ErrNoResponse
		}
		go func() {
			serr <- err
		}()
		return
	}

	srcw := io.MultiWriter(res, c.Out())
	dres.Write(srcw)
}

//StreamTLSWith handles streaming two http conns
func (h *TCP) StreamTLSWith(src net.Conn, addr string) (*ConnInsight, error) {
	rsrc := NewStreamConn(src)
	dest, err := NewTLSFromConn(src, addr)
	if err != nil {
		return nil, err
	}
	return h.Stream(rsrc, dest)
}

//StreamTypeConn handles streaming two http conns
func (h *TCP) StreamTypeConn(src net.Conn, addr, ts string, conf *tls.Config) (*ConnInsight, error) {
	rsrc := NewStreamConn(src)
	rdest, err := NewTypeConn(addr, ts, conf)

	if err != nil {
		return nil, err
	}

	return h.Stream(rsrc, rdest)
}

//StreamWithConn handles streaming two http conns
func (h *TCP) StreamWithConn(src net.Conn, addr string, conf *tls.Config) (*ConnInsight, error) {
	rsrc := NewStreamConn(src)

	var rdest *StreamConn
	var err error

	if conf == nil {
		rdest, err = NewNetConn(addr)
	} else {
		rdest, err = NewTLSConn(addr, conf)
	}

	if err != nil {
		return nil, err
	}

	return h.Stream(rsrc, rdest)
}

//StreamWithConn handles streaming two http conns with an optional tls.Config
func (h *HTTP) StreamWithConn(src net.Conn, addr string, conf *tls.Config) (*ConnInsight, error) {
	rsrc := NewStreamConn(src)

	var rdest *StreamConn
	var err error

	if conf == nil {
		rdest, err = NewNetConn(addr)
	} else {
		rdest, err = NewTLSConn(addr, conf)
	}

	if err != nil {
		return nil, err
	}

	return h.Stream(rsrc, rdest)
}

//StreamConns handles streaming two http conns
func (h *HTTP) StreamConns(src, dest net.Conn) (*ConnInsight, error) {
	return h.Stream(NewStreamConn(src), NewStreamConn(dest))
}

//StreamReq handles streaming between a net.Conn and a addr
func (h *HTTP) StreamReq(src net.Conn, addr string) (*ConnInsight, error) {
	rsrc := NewStreamConn(src)
	dest, err := TargetReqRes(addr)

	if err != nil {
		return nil, err
	}

	return h.Stream(rsrc, dest)
}

//StreamUnit handles streaming between a http Request,ReponseWriter and a addr
func (h *HTTP) StreamUnit(req *http.Request, res http.ResponseWriter, addr string) (*ConnInsight, error) {
	rsrc := NewReqRes(res, req)
	dest, err := TargetReqRes(addr)

	if err != nil {
		return nil, err
	}

	return h.Stream(rsrc, dest)
}

//StreamUnitConn handles streaming between a http Request,ReponseWriter and a net.Conn
func (h *HTTP) StreamUnitConn(req *http.Request, res http.ResponseWriter, dest net.Conn) (*ConnInsight, error) {
	rsrc := NewReqRes(res, req)
	dc := NewStreamConn(dest)

	return h.Stream(rsrc, dc)
}

//StreamUnitAddr handles streaming between a http Request,ReponseWriter and a net.Conn,this tls.Config can be left as nil to make a normal net.Conn destination
func (h *HTTP) StreamUnitAddr(req *http.Request, res http.ResponseWriter, dest string, conf *tls.Config) (*ConnInsight, error) {
	rsrc := NewReqRes(res, req)

	var dc *StreamConn
	var err error

	if conf == nil {
		dc, err = NewNetConn(dest)
	} else {
		dc, err = NewTLSConn(dest, conf)
	}

	if err != nil {
		return nil, err
	}

	return h.Stream(rsrc, dc)
}

//StreamTLSWith handles streaming two http conns
func (h *HTTP) StreamTLSWith(src net.Conn, addr string) (*ConnInsight, error) {
	rsrc := NewStreamConn(src)
	dest, err := NewTLSFromConn(src, addr)
	if err != nil {
		return nil, err
	}
	return h.Stream(rsrc, dest)
}

//StreamReqRes handles streaming two http conns
func (h *HTTP) StreamReqRes(req *http.Request, res http.ResponseWriter, addr string) (*ConnInsight, error) {
	src := NewReqRes(res, req)
	dest, err := TargetReqRes(addr)

	if err != nil {
		return nil, err
	}

	return h.Stream(src, dest)
}

//HTTPStream handles http proxy processes
func HTTPStream(he ErrorHandler) *HTTP {
	return &HTTP{
		NewProxyStream(func(c *ConnInsight, s NotifierError) {

			src := c.Src()
			dest := c.Dest()

			//check if the using the src as the key type

			//are they both net.Conns
			_, so := src.(*StreamConn)
			_, do := dest.(*StreamConn)

			if so && do {
				handleHTTPConns(c, s)
				return
			}

			//is a pair of dest StreamConn and src ReqResConn
			_, sod := src.(*ReqResConn)

			if do && sod {
				handleNet2HTTP(c, s)
				return
			}

			//are they one net.Conn and a url to the destination
			_, do = dest.(*ReqResConn)

			if do && so {
				handleHTTP2Net(c, s)
				return
			}

			//are they both reqres type(that is a reqres was recieved with a url pointing to the destination)
			_, so = src.(*ReqResConn)

			if do && so {
				handleHTTPReqRes(c, s)
				return
			}

			go func() {
				s <- ErrBadConn
			}()

		}, he),
	}
}

//TCPStream handles http proxy processes
func TCPStream(he ErrorHandler) *TCP {
	return &TCP{
		NewProxyStream(func(c *ConnInsight, se NotifierError) {

			dest := c.Dest()
			src := c.Src()

			if dest == nil || src == nil {
				log.Println("Invalid Connections")
				go func() {
					se <- ErrBadConn
				}()
				return
			}

			ws := new(sync.WaitGroup)
			ws.Add(2)

			rdest := dest.Reader()
			wdest := dest.Writer()

			rsrc := src.Reader()
			wsrc := src.Writer()

			destwriter := NopWriter(io.MultiWriter(wdest, c.In()))
			srcwriter := NopWriter(io.MultiWriter(wsrc, c.Out()))

			flux.GoDefer("connCloser", func() {
				ws.Wait()
				c.Close()
			})

			flux.GoDefer("dest2src", func() {
				log.Printf("Copying to destination for tcp")
				_, ex := io.Copy(destwriter, rsrc)

				if ex != nil {
					go func() { se <- ex }()
				}
				ws.Done()
			})

			flux.GoDefer("src2dest", func() {
				log.Printf("Copying to src for tcp")
				_, ex := io.Copy(srcwriter, rdest)

				if ex != nil {
					go func() { se <- ex }()
				}
				ws.Done()
			})

		}, he),
	}
}

//NewDirector returns a new director instance
func NewDirector(max, hl time.Duration, hle ErrorHandler) *DirectorFactory {
	d := &Director{
		errors:      make(NotifierError),
		closer:      make(Notifier),
		health:      make(Notifier),
		do:          new(sync.Once),
		init:        new(sync.Once),
		halter:      new(sync.Once),
		resumer:     nil,
		tcp:         TCPStream(hle),
		http:        HTTPStream(hle),
		maxage:      max,
		healthCheck: hl,
		errHandler:  hle,
		connectors:  make(map[string]Closers),
		requests:    flux.NewIdentityStream(),
		waiter:      new(sync.WaitGroup),
	}

	dd := NewFactory(d)
	// addFactoryDefaults(dd)
	return dd
}

//Init handles the process of the director
func (d *Director) Init() {
	d.init.Do(func() {
		go func() {
		dloop:
			for {
				select {
				case <-time.After(d.healthCheck):
					log.Println("Sending healthcheck signal")
					go func() { d.health <- struct{}{} }()
				case err := <-d.errors:
					log.Println("Received error", err)
					if d.errHandler != nil {
						d.errHandler(err)
					}
				case <-d.closer:
					log.Println("Received closed signal")
					break dloop
				}
			}
		}()
	})
}

//MaxAge returns a duration when a connection can
//still be active and the connection can decide to
//self die(harakiri itself) if its being idle beyond this age
func (d *Director) MaxAge() time.Duration {
	return d.maxage
}

//Errors returns a channel that can be used to notify close of operation
func (d *Director) Errors() (NotifierError, bool) {
	if d.do == nil {
		return d.errors, false
	}
	return d.errors, true
}

//HttpStream returns the director http proxystreamer
func (d *Director) HttpStream() HTTPStreamer {
	return d.http
}

//TcpStream returns the director tcp proxystreamer
func (d *Director) TcpStream() TCPStreamer {
	return d.tcp
}

//ServeTCP wraps the creation of a ConnServe and binds the director for easy management and serves a (optional tls or normal) bare net.Conn tcp connection (i.e using tls with a base net.Conn and not a http.Server)
func (d *Director) ServeTCP(a Action, from, to string, conf *tls.Config) error {
	return d.ServeCustomTCP(from, conf, func(con net.Conn, dir Directors) error {

		var c *ConnInsight
		var err error

		c, err = dir.TcpStream().StreamWithConn(con, to, conf)

		if err != nil {
			return err
		}

		a(c)
		return nil
	})
}

//ServeTlsTCP provides a special case when the connection must no doubt be as tcp net.Conn using a tsl.Config else turn to extracting the tls from the src net.Conn
func (d *Director) ServeTlsTCP(a Action, from, to string, conf *tls.Config) error {
	return d.ServeCustomTCP(from, conf, func(con net.Conn, dir Directors) error {

		var c *ConnInsight
		var err error

		if conf != nil {
			c, err = dir.TcpStream().StreamWithConn(con, to, conf)
		} else {
			c, err = dir.TcpStream().StreamTLSWith(con, to)
		}

		if err != nil {
			return err
		}

		a(c)
		return nil
	})
}

//ServeConn wraps the creation of a ConnServe and binds the director for easy management for handling generic to be provided net.Conn with specific type
func (d *Director) ServeConn(a Action, from, to, ts string, conf *tls.Config) error {
	return d.ServeCustomConn(from, ts, func(con net.Conn, dir Directors) error {

		c, err := dir.TcpStream().StreamTypeConn(con, to, ts, conf)

		if err != nil {
			return err
		}

		a(c)
		return nil
	}, conf)
}

//ServeHTTPConn provides a convenient provider for when dealing with raw http net.Conns alone without mixing in of http.Request,using the provided http proxy net.Conn centric functions. The last argument toReq (boolean) tells the server to if true treat the connection as going from a net.Conn to http.Request(that is use a http.Request to proxy) and if false instead use a http net.Conn for both request
func (d *Director) ServeHTTPConn(a Action, from, to string, toReq bool) error {
	return d.ServeCustomHTTPConns(from, nil, func(con net.Conn, dir Directors) error {
		var c *ConnInsight
		var err error

		if !toReq {
			c, err = dir.HttpStream().StreamWithConn(con, to, nil)
		} else {
			c, err = dir.HttpStream().StreamReq(con, to)
		}

		if err != nil {
			return err
		}

		a(c)
		return nil
	})
}

//ServeTLSHTTP provides a convenient provider for when dealing with raw http net.Conns alone without mixing in of http.Request,using the provided http proxy net.Conn centric functions,if a *tls.Config is supplied it uses that for all connection,else it takes the connection it gets and treat it as a tls.Conn then tries to extract the certificates to create a new tls.Conn to the destination
func (d *Director) ServeTLSHTTP(a Action, from, to string, conf *tls.Config) error {
	return d.ServeCustomHTTPConns(from, conf, func(con net.Conn, dir Directors) error {
		var c *ConnInsight
		var err error

		if conf != nil {
			c, err = dir.HttpStream().StreamWithConn(con, to, conf)
		} else {
			c, err = dir.HttpStream().StreamTLSWith(con, to)
		}

		if err != nil {
			return err
		}

		a(c)
		return nil
	})
}

//ServeHTTP wraps the creation of a httpServe and binds the director for easy management. It provides dual use for two types of mechanism: 																	1. handling streaming from one http.Request to another(the destination)																							2. handling streaming from a http.Request to a net.Conn http connection																							This type are determined by the toCon bool switch,when true it turns into option 2 operation and when false turns to operation 1
func (d *Director) ServeHTTP(a Action, from, to string, toCon bool, cf *tls.Config) error {
	return d.ServeCustomHTTP(from, cf, func(res http.ResponseWriter, req *http.Request, dir Directors) error {

		var c *ConnInsight
		var err error

		if toCon {
			c, err = dir.HttpStream().StreamUnitAddr(req, res, to, cf)
		} else {
			c, err = dir.HttpStream().StreamUnit(req, res, to)
		}

		if err != nil {
			return err
		}

		a(c)
		return nil
	})
}

//ServeReqRes is a special case for proxying between two individual http.Request and their response, using the ReqRes to ReqRes strategy, this is just created for convenience and for those who prefer such a high level proxy approach to the low-level http net.Conn to http net.Conn provided by Director.ServeHTTPConn
func (d *Director) ServeReqRes(a Action, from, to string) error {
	return d.ServeCustomHTTP(from, nil, func(res http.ResponseWriter, req *http.Request, dir Directors) error {

		var c *ConnInsight
		var err error

		c, err = dir.HttpStream().StreamReqRes(req, res, to)

		if err != nil {
			return err
		}

		a(c)
		return nil
	})
}

//ServeCustomConn provides a means of creating a custom ConnServe for tcp connections into the director for managememnt i.e create net.Listener and use its generic net.Conn
func (d *Director) ServeCustomConn(addr, ctype string, t TargetOp, conf *tls.Config) error {
	_, ok := d.connectors[addr]

	if ok {
		return ErrKeyUsed
	}

	hs, err := ServeType(t, d, addr, ctype, conf)

	if err != nil {
		return err
	}

	d.connectors[addr] = hs
	return nil
}

//ServeCustomTCP provides a means of creating a custom ConnServe for tcp connections into the director for managememnt i.e create net.Listener and use its generic net.Conn which is a net.TCPListener
func (d *Director) ServeCustomTCP(addr string, conf *tls.Config, t TargetOp) error {
	_, ok := d.connectors[addr]

	if ok {
		return ErrKeyUsed
	}

	var hs *ConnServe
	var err error

	if conf != nil {
		hs, err = ServeTLS(t, d, addr, conf)
	} else {
		hs, err = Serve(t, d, addr)
	}

	if err != nil {
		return err
	}

	d.connectors[addr] = hs
	return nil
}

//ServeCustomHTTP provides a means of creating a custom httpServe into the director for management for reqres http requests i.e tie itself to a http.Server and use http.Request and http.ResponseWriter
func (d *Director) ServeCustomHTTP(addr string, conf *tls.Config, t TargetReqResOp) error {
	_, ok := d.connectors[addr]

	if ok {
		return ErrKeyUsed
	}

	hs, err := ServeHTTPWith(t, addr, d, conf)

	if err != nil {
		return err
	}

	d.connectors[addr] = hs
	return nil
}

//ServeCustomHTTPConns provides a means of creating a custom httpServe into the director for management of http net.Conn i.e create net.Listener issues a http net.Conn
func (d *Director) ServeCustomHTTPConns(addr string, conf *tls.Config, t TargetOp) error {
	_, ok := d.connectors[addr]

	if ok {
		return ErrKeyUsed
	}

	hs, err := ServeBase(t, d, addr, conf)

	if err != nil {
		return err
	}

	d.connectors[addr] = hs
	return nil
}

//CloseConnector provides a method of closing a connection procesor
func (d *Director) CloseConnector(addr string) {
	c, ok := d.connectors[addr]

	if !ok {
		return
	}

	delete(d.connectors, addr)
	c.Close()
}

//Wait allows servers to check if the director is blocking
func (d *Director) Wait() {
	d.waiter.Wait()
}

//Pause adds to the director WaitGroup which blocks all
//servers that use it for sync
func (d *Director) Pause() {
	d.halter.Do(func() {
		d.waiter.Add(1)
		d.resumer = new(sync.Once)
	})
}

//Resume completes the waiter for release the waiter
func (d *Director) Resume() {
	if d.resumer == nil {
		return
	}

	defer func() {
		d.resumer = nil
	}()

	d.resumer.Do(func() {
		d.waiter.Done()
		d.halter = new(sync.Once)
	})
}

//Requests returns a streamer which notifiers of a request coming in
func (d *Director) Requests() flux.StackStreamers {
	return d.requests
}

//Close stops the director and all connected servers
func (d *Director) Close() {
	defer func() {
		d.init = new(sync.Once)
		d.do = new(sync.Once)
		d.http = nil
		d.tcp = nil
	}()
	d.do.Do(func() {
		d.tcp.Close()
		d.http.Close()
		close(d.closer)
	})
}

//CloseNotify returns a channel that can be used to notify close of operation
func (d *Director) CloseNotify() Notifier {
	return d.closer
}

//HealthNotify returns a channel thats used to request
//all users to check their healthcondition(eg idle times)
func (d *Director) HealthNotify() Notifier {
	return d.health
}

//Close closes the connserve and stop all operations
func (c *ConnServe) Close() {
	c.do.Do(func() {
		close(c.closer)
	})
}

//handleOperations manages the operations and behaviours of the connserver
func (c *ConnServe) handleOperations() {
	hb := make(chan struct{})
	close(hb)
	go func() {
		defer c.listener.Close()
	connloop:
		for {
			select {
			case <-c.director.HealthNotify():
				age := c.director.MaxAge()
				idle := time.Duration(c.idle.Unix())
				if idle > age {
					break connloop
				}
			case <-c.closer:
				hb = nil
				break connloop
			case <-c.director.CloseNotify():
				hb = nil
				break connloop
			case <-hb:
				con, err := c.listener.Accept()
				errs, ok := c.director.Errors()

				if err != nil {
					if !ok {
						go func() {
							errs <- err
						}()
					}
					return
				}

				c.idle = time.Now()

				//send the process in to a goroutine,lets not block
				// err = c.target(con, c.director)
				c.director.Requests().Emit(true)
				go func() {
					c.director.Wait()
					err := c.target(con, c.director)
					if err != nil {
						if !ok {
							go func() {
								errs <- err
							}()
						}
					}
				}()

			}
		}
	}()
}

//Close ends the httpserve operation
func (hs *HTTPServe) Close() {
	hs.do.Do(func() {
		close(hs.closer)
	})
}

//NewConServe returns a new connection server
func NewConServe(l net.Listener, d Directors, t TargetOp) *ConnServe {
	c := &ConnServe{
		listener: l,
		director: d,
		idle:     time.Now(),
		target:   t,
		do:       new(sync.Once),
		closer:   make(Notifier),
	}

	c.handleOperations()

	return c
}

//ServeBase returns a new ConnServe using an addr and an optional tls.Config
func ServeBase(t TargetOp, d Directors, addr string, conf *tls.Config) (*ConnServe, error) {
	l, err := MakeBaseListener(addr, conf)

	if err != nil {
		return nil, err
	}

	return NewConServe(l, d, t), nil
}

//ServeType returns a new ConnServe using an addr
func ServeType(t TargetOp, d Directors, addr, ft string, cf *tls.Config) (*ConnServe, error) {
	l, err := MakeListener(addr, ft, cf)

	if err != nil {
		return nil, err
	}

	return NewConServe(l, d, t), nil
}

//Serve returns a new ConnServe using addr and port
func Serve(t TargetOp, d Directors, addr string) (*ConnServe, error) {
	return ServeBase(t, d, addr, nil)
}

//ServeTLSFrom returns a new ConnServe using addr
func ServeTLSFrom(t TargetOp, d Directors, ft, addr string, conf *tls.Config) (*ConnServe, error) {
	l, err := tls.Listen(ft, addr, conf)
	if err != nil {
		return nil, err
	}
	return NewConServe(l, d, t), nil
}

//ServeTLS returns a new ConnServe using addr
func ServeTLS(t TargetOp, d Directors, addr string, conf *tls.Config) (*ConnServe, error) {
	return ServeTLSFrom(t, d, "tcp", addr, conf)
}

//ServeHTTPWith provides a different approach,instead of using the base net.Conn ,itself uses the http.Request and http.Response as means of proxying using the director
func ServeHTTPWith(t TargetReqResOp, addr string, d Directors, conf *tls.Config) (*HTTPServe, error) {
	var hs *http.Server
	var ls net.Listener
	var err error

	handler := http.HandlerFunc(func(res http.ResponseWriter, req *http.Request) {
		d.Requests().Emit(true)
		go func() {
			d.Wait()
			err := t(res, req, d)
			if err != nil {
				eos, ok := d.Errors()
				if ok {
					go func() {
						eos <- err
					}()
				}
			}
		}()
	})

	if conf != nil {
		hs, ls, err = CreateTLS(addr, conf, handler)
	} else {
		hs, ls, err = CreateHTTP(addr, handler)
	}

	if err != nil {
		return nil, err
	}

	hps := &HTTPServe{
		listener: ls,
		server:   hs,
		director: d,
		idle:     time.Now(),
		closer:   make(Notifier),
	}

	go func() {
		defer ls.Close()
	nloop:
		for {
			select {
			case <-d.HealthNotify():
				age := hps.director.MaxAge()
				idle := time.Duration(hps.idle.Unix())
				if idle > age {
					break nloop
				}
			case <-hps.closer:
				break nloop
			}
		}
	}()

	return hps, nil
}

//ServeHTTP returns a HTTPServe for http ReqRes proxying
func ServeHTTP(t TargetReqResOp, d Directors, addr string) (*HTTPServe, error) {
	return ServeHTTPWith(t, addr, d, nil)
}

//NewFactory returns a new DirectorFactory
func NewFactory(d *Director) *DirectorFactory {
	return &DirectorFactory{
		Director: d,
		factory:  flux.NewSecureMap(),
	}
}

//Build takes a map of ProxyRequest and builds them with its builders into the Directory
func (c *DirectorFactory) Build(p Proxies, a Action, fx func(*ProxyRequest)) {
	for k, v := range p {
		c.Make(k, v, a, fx)
	}
}

//Make takes a key and map and buiilds a proxy for it
func (c *DirectorFactory) Make(k string, v *ProxyRequest, a Action, fx func(*ProxyRequest)) {
	v.From = k
	bind, err := c.Find(v.Type)
	if err == nil {
		err = bind(v, c.Director, func(cx *ConnInsight) {
			cx.Type = k
			cx.Req = v
			a(cx)
		})
		if err != nil && c.errHandler != nil {
			c.errHandler(err)
		}

		if fx != nil {
			fx(v)
		}
	}
}

//Find lets you get a new condition maker
func (c *DirectorFactory) Find(tag string) (DirectorBinders, error) {
	if !c.factory.Has(tag) {
		return nil, ErrorNotFind
	}
	return c.factory.Get(tag).(DirectorBinders), nil
}

//Register lets you add a new condition maker
func (c *DirectorFactory) Register(tag string, fx DirectorBinders) {
	c.factory.Set(tag, fx)
}

//Deregister lets you add a new condition maker
func (c *DirectorFactory) Deregister(tag string) {
	c.factory.Remove(tag)
}

//Direct is just a convenience method
func Direct(age, check time.Duration, he ErrorHandler) *DirectorFactory {
	return NewDirector(age, check, he)
}

type (

	//ProxyDirector defines a proxie interface methods
	ProxyDirector interface {
		Factory
		ServeCustomHTTPConns(addr string, conf *tls.Config, t TargetOp) error
		ServeCustomHTTP(addr string, conf *tls.Config, t TargetReqResOp) error
		ServeCustomTCP(addr string, conf *tls.Config, t TargetOp) error
		ServeCustomConn(addr, ctype string, t TargetOp, conf *tls.Config) error
		ServeReqRes(a Action, from, to string) error
		ServeHTTP(a Action, from, to string, toCon bool, cf *tls.Config) error
		ServeTLSHTTP(a Action, from, to string, conf *tls.Config) error
		ServeHTTPConn(a Action, from, to string, toReq bool) error
		ServeConn(a Action, from, to, ts string, conf *tls.Config) error
		ServeTlsTCP(a Action, from, to string, conf *tls.Config) error
		ServeTCP(a Action, from, to string, conf *tls.Config) error
		CloseConnector(string)
	}
	//Factory provides a nice interface for DirectorFactory
	Factory interface {
		Directors
		Closers
		Find(string) (DirectorBinders, error)
		Register(string, DirectorBinders)
		Deregister(tag string)
		Build(Proxies, Action, func(*ProxyRequest))
		Make(string, *ProxyRequest, Action, func(*ProxyRequest))
	}

	//DirectorBinders provides a the binder worker for a connection type
	DirectorBinders func(*ProxyRequest, *Director, Action) error

	//DirectorFactory provides a nice means of registering action binders for directors
	DirectorFactory struct {
		*Director
		factory *flux.SecureMap
	}

	//TargetReqResOp defines a function type taking a target http.Request and http.ResponseWriter with a Director
	TargetReqResOp func(http.ResponseWriter, *http.Request, Directors) error

	//TargetOp defines a function type taking a target net.Conn and Director
	TargetOp func(net.Conn, Directors) error

	//Action is a type for when a ConnInsight is recieved
	Action func(*ConnInsight)

	//Closers provide a simple close() method interface
	Closers interface {
		Close()
	}

	//Directors provides an interface defining conn directors method rules
	Directors interface {
		Requests() flux.StackStreamers
		MaxAge() time.Duration
		HealthNotify() Notifier
		Errors() (NotifierError, bool)
		CloseNotify() Notifier
		TcpStream() TCPStreamer
		HttpStream() HTTPStreamer
		Wait()
		Init()
		Pause()
		Resume()
	}

	//Director provides a combination of connection management and streaming, creating inner or managing provided persistent connections and proxying each request as per given rule
	Director struct {
		tcp         *TCP
		http        *HTTP
		closer      Notifier
		health      Notifier
		do          *sync.Once
		halter      *sync.Once
		resumer     *sync.Once
		init        *sync.Once
		errors      NotifierError
		maxage      time.Duration
		healthCheck time.Duration
		errHandler  ErrorHandler
		waiter      *sync.WaitGroup
		requests    flux.StackStreamers
		connectors  map[string]Closers
	}

	//ConnServe is a connection process for the low level net.Listener
	ConnServe struct {
		listener net.Listener
		director Directors
		idle     time.Time
		target   TargetOp
		do       *sync.Once
		closer   Notifier
	}

	//HTTPServe is a connection processor for a httpserver
	HTTPServe struct {
		listener net.Listener
		server   *http.Server
		director Directors
		idle     time.Time
		closer   Notifier
		do       *sync.Once
	}

	//ReqResConn provides a structural wrapper over http ReqRes objects
	ReqResConn struct {
		*BaseConn
		Res  http.ResponseWriter
		Req  *http.Request
		Addr string
	}

	//StreamConn provides structural wrapping over your proxy connection target
	StreamConn struct {
		*BaseConn
		src net.Conn
	}

	//BaseConn defines the base structural of a Conn
	BaseConn struct {
		closer Notifier
		// errorer NotifierError
		doend *sync.Once
	}

	//Conn provides a facade over for handling connection with netstream
	Conn interface {
		CloseNotify() Notifier
		Reader() io.ReadCloser
		Writer() io.WriteCloser
		Src() interface{}
		Close() error
	}

	//ConnInsight provides a stackstream for listening into the sending and recieving ops of the Conn
	ConnInsight struct {
		Type            string
		Req             *ProxyRequest
		Meta            map[string]string
		in, out, closed flux.StackStreamers
		src, dest       Conn
	}

	//HTTP handles all http proxystreams
	HTTP struct {
		ProxyStreams
	}

	//TCPStreamer provides interface method spec for TCP proxies
	TCPStreamer interface {
		CoverStreamers
		StreamTLSWith(net.Conn, string) (*ConnInsight, error)
		StreamWithConn(net.Conn, string, *tls.Config) (*ConnInsight, error)
		StreamTypeConn(net.Conn, string, string, *tls.Config) (*ConnInsight, error)
	}

	//HTTPStreamer provides interface method spec for HTTP proxies
	HTTPStreamer interface {
		CoverStreamers
		StreamReqRes(*http.Request, http.ResponseWriter, string) (*ConnInsight, error)
		StreamUnit(*http.Request, http.ResponseWriter, string) (*ConnInsight, error)
		StreamUnitAddr(*http.Request, http.ResponseWriter, string, *tls.Config) (*ConnInsight, error)
		StreamUnitConn(*http.Request, http.ResponseWriter, net.Conn) (*ConnInsight, error)
		StreamWithConn(net.Conn, string, *tls.Config) (*ConnInsight, error)
		StreamReq(net.Conn, string) (*ConnInsight, error)
		StreamTLSWith(net.Conn, string) (*ConnInsight, error)
		StreamConns(net.Conn, net.Conn) (*ConnInsight, error)
	}

	//TCP handles all tcp net.Conn proxystreams
	TCP struct {
		ProxyStreams
	}

	//ErrorHandler provides the error handler type
	ErrorHandler func(error)

	//Notifier provides a nice type for a channel for struct{}
	Notifier chan struct{}

	//NotifierError provides a nice type for error chan
	NotifierError chan error

	//ConnWork represent a channel of net.Conn requests
	ConnWork chan net.Conn

	//WorkNotifier handles managing of workload
	WorkNotifier chan *ConnInsight

	// ProxyStream provides a basic level streaming connections
	ProxyStream struct {
		closer       Notifier
		errors       NotifierError
		work         WorkNotifier
		addWorker    chan int64
		endWorker    chan int64
		do           *sync.Once
		processor    ConnectionProcess
		handleErrors ErrorHandler
		workers      int64
		resumer      *sync.Once
		halter       *sync.Once
		waiter       *sync.WaitGroup
	}

	//ConnectionProcess provides a type of net.Conn creation
	ConnectionProcess func(*ConnInsight, NotifierError)

	//CoverStreamers define the safe method rules of proxies
	CoverStreamers interface {
		Closed() bool
		Stream(Conn, Conn) (*ConnInsight, error)
		AddWorker()
		RemoveWorker()
	}

	//ProxyStreams defines the streaming api member functions for streams,it allows the addition of workers and removal of all workers except one i.e you can add workers but the last worker wont stop unless you close the streamer
	ProxyStreams interface {
		CoverStreamers
		Close() error
		GetProcessor() ConnectionProcess
	}

	//Nopwriter provides a writer with a Close member func
	Nopwriter struct {
		io.Writer
	}

	//ProxyRequest defines a proxy configuration
	ProxyRequest struct {
		Type     string `yaml:"type" json:"type"`
		Port     int    `yaml:"port" json:"port"`
		Addr     string `yaml:"addr" json:"addr"`
		CertFile string `yaml:"cert" json:"cert"`
		KeyFile  string `yaml:"key" json:"key"`
		From     string `yaml:"-" json:"-"`
	}

	//Proxies represent a provision for handling proxy information
	Proxies map[string]*ProxyRequest
)

var (
	//httpClient is a default http client used for making request
	httpclient = &http.Client{}
	//ErrKeyUsed defines when an ip address and port is already be used by a director and a existing connection processor
	ErrKeyUsed = errors.New("the addr is already assigned")
	//ErrBadStreamer represents an error returned when a stream is incapable of operation
	ErrBadStreamer = errors.New("BadStreamer")
	//ErrNoRequest represent when a nil request is received
	ErrNoRequest = errors.New("Invalid! No Request Received!")
	//ErrNoResponse represents when a nil response is recieved
	ErrNoResponse = errors.New("Invalid Conn! No Response Recieved!")
	//ErrBadConn represents when conn is not valid or type not verifiable
	ErrBadConn = errors.New("Invalid Conn!")
	// Hop-by-hop headers. These are removed when sent to the backend. http://www.w3.org/Protocols/rfc2616/rfc2616-sec13.html
	hopHeaders = []string{
		"Connection",
		"Keep-Alive",
		"Proxy-Authenticate",
		"Proxy-Authorization",
		"Te", // canonicalized version of "TE"
		"Trailers",
		"Transfer-Encoding",
		"Upgrade",
	}
)
