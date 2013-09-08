package main

import (
	"encoding/json"
	"fmt"
	"io"
	"labix.org/v2/mgo"
	"labix.org/v2/mgo/bson"
	. "launchpad.net/gocheck"
	"net"
	"net/http"
	"net/url"
	"testing"
	"time"
)

// Hook up gocheck into the "go test" runner.
func Test(t *testing.T) { TestingT(t) }

type WebAPISuite struct {
	WebRoot string
	Session *mgo.Session
}

var _ = Suite(&WebAPISuite{})

func (s *WebAPISuite) SetUpSuite(c *C) {
	config, err := ConfigOpen(*configPath)
	if err != nil {
		c.Fatal(err)
	}

	// Set session timeout to fail early and avoid long response times.
	s.Session, err = mgo.DialWithTimeout(config.Mongo.URL, 5*time.Second)
	if err != nil {
		c.Fatal("[MongoDB]", err)
	}

	db = s.Session.DB(config.Mongo.DB + "_test")
	// Drop all collections instead of dropping the database to avoid
	// reallocating the database file on each run
	names, err := db.CollectionNames()
	if err != nil {
		c.Fatal(err)
	}
	for _, name := range names {
		db.C(name).DropCollection()
	}

	// Listen on any available port assigned by the system
	s.listenAndServe("localhost:0", APIHandler(), c)
}

func (s *WebAPISuite) TearDownSuite(c *C) {
	s.Session.Close()
}

func (s *WebAPISuite) listenAndServe(addr string, handler http.Handler, c *C) {
	l, err := net.Listen("tcp", addr)
	if err != nil {
		c.Fatal(err)
	}
	srv := &http.Server{
		Addr:    addr,
		Handler: handler,
	}
	s.WebRoot = l.Addr().String()
	go func() {
		c.Fatal(srv.Serve(l))
	}()
}

// postForm is a helper to http.PostForm that prepends the address of the test server to form the URL.
func (s *WebAPISuite) postForm(path string, data url.Values) (resp *http.Response, err error) {
	url := fmt.Sprintf("http://%s%s", s.WebRoot, path)
	return http.PostForm(url, data)
}

type Response struct {
	Ack        *SessionAck
	StatusCode int
}

func (s *WebAPISuite) newSession(jid, machineId, xmppvoxVersion string) (r *Response, err error) {
	data := url.Values{}
	data.Set("jid", jid)
	data.Set("machine_id", machineId)
	data.Set("xmppvox_version", xmppvoxVersion)
	resp, err := s.postForm("/1/session/new", data)
	r = &Response{StatusCode: resp.StatusCode}
	if err != nil {
		return
	}
	defer resp.Body.Close()
	dec := json.NewDecoder(resp.Body)
	r.Ack = &SessionAck{}
	if err = dec.Decode(r.Ack); err != io.EOF && err != nil {
		return
	}
	return
}

func (s *WebAPISuite) closeSession(id bson.ObjectId) (r *Response, err error) {
	data := url.Values{}
	data.Set("id", id.Hex())
	resp, err := s.postForm("/1/session/close", data)
	r = &Response{StatusCode: resp.StatusCode}
	if err != nil {
		return
	}
	defer resp.Body.Close()
	dec := json.NewDecoder(resp.Body)
	r.Ack = &SessionAck{}
	if err = dec.Decode(r.Ack); err != io.EOF && err != nil {
		return
	}
	return
}

// ************************ Tests ************************

func (s *WebAPISuite) TestNewSession(c *C) {
	const (
		jid            = "testuser@server.org"
		machineId      = "00:26:cc:18:be:14"
		xmppvoxVersion = "1.0"
	)
	r, err := s.newSession(jid, machineId, xmppvoxVersion)
	c.Assert(err, IsNil)
	c.Check(r.Ack.Status, Equals, "ok")
	session := &Session{}
	err = db.C("sessions").FindId(r.Ack.Id).One(session)
	c.Assert(err, IsNil)
	c.Check(session.CreatedAt.IsZero(), Equals, false)
	c.Check(session.ClosedAt.IsZero(), Equals, true)
	c.Check(session.JID, Equals, jid)
	c.Check(session.MachineId, Equals, machineId)
	c.Check(session.XMPPVOXVersion, Equals, xmppvoxVersion)
	c.Check(session.Request, NotNil)
}

func (s *WebAPISuite) TestNewSessionMissingFields(c *C) {
	countBefore, err := db.C("sessions").Find(nil).Count()
	c.Assert(err, IsNil)
	type TestCase struct {
		JID, MachineId, XMPPVOXVersion string
	}
	for _, tc := range []TestCase{
		TestCase{"", "00:26:cc:18:be:14", "1.0"},
		TestCase{"testuser@server.org", "", "1.0"},
		TestCase{"testuser@server.org", "00:26:cc:18:be:14", ""},
		TestCase{"testuser@server.org", "", ""},
		TestCase{"", "", ""},
	} {
		_, err := s.newSession(tc.JID, tc.MachineId, tc.XMPPVOXVersion)
		c.Assert(err.Error(), Matches, "400 .*")
	}
	countAfter, err := db.C("sessions").Find(nil).Count()
	c.Assert(err, IsNil)
	c.Check(countAfter, Equals, countBefore)
}

func (s *WebAPISuite) TestCloseSession(c *C) {
	nr, err := s.newSession("testuser@server.org", "00:26:cc:18:be:14", "1.0")
	c.Assert(err, IsNil)
	c.Assert(nr.Ack.Status, Equals, "ok")
	cr, err := s.closeSession(nr.Ack.Id)
	c.Assert(err, IsNil)
	c.Check(cr.Ack.Status, Equals, "ok")
}
