package main

import (
	"encoding/json"
	"fmt"
	"io/ioutil"
	"labix.org/v2/mgo"
	"labix.org/v2/mgo/bson"
	. "launchpad.net/gocheck"
	"net"
	"net/http"
	"net/url"
	"strings"
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
	Body       string
	StatusCode int
}

func (s *WebAPISuite) apiPostCall(apiResource string, data map[string]string) (r *Response, err error) {
	postData := url.Values{}
	for key, value := range data {
		postData.Set(key, value)
	}
	resp, err := s.postForm(apiResource, postData)
	if err != nil {
		return
	}
	r = &Response{StatusCode: resp.StatusCode}
	defer resp.Body.Close()
	b, err := ioutil.ReadAll(resp.Body)
	r.Body = string(b)
	return
}

func (s *WebAPISuite) newInstallation(machineId, xmppvoxVersion string, dosvoxInfo, machineInfo map[string]string) (r *Response, err error) {
	m := func(v interface{}) string {
		b, _ := json.Marshal(v)
		return string(b)
	}
	return s.apiPostCall("/1/installation/new", map[string]string{
		"machine_id":      machineId,
		"xmppvox_version": xmppvoxVersion,
		"dosvox_info":     m(dosvoxInfo),
		"machine_info":    m(machineInfo),
	})
}

func (s *WebAPISuite) newSession(jid, machineId, xmppvoxVersion string) (r *Response, err error) {
	return s.apiPostCall("/1/session/new", map[string]string{
		"jid":             jid,
		"machine_id":      machineId,
		"xmppvox_version": xmppvoxVersion,
	})
}

func (s *WebAPISuite) closeSession(sessionId bson.ObjectId, machineId string) (r *Response, err error) {
	return s.apiPostCall("/1/session/close", map[string]string{
		"session_id": sessionId.Hex(),
		"machine_id": machineId,
	})
}

func (s *WebAPISuite) pingSession(sessionId bson.ObjectId, machineId string) (r *Response, err error) {
	return s.apiPostCall("/1/session/ping", map[string]string{
		"session_id": sessionId.Hex(),
		"machine_id": machineId,
	})
}

// ************************ Tests ************************

// Install tests

func (s *WebAPISuite) TestNewInstallation(c *C) {
	const (
		machineId      = "0e5ab64c-1b24-4917-bb9e-new-installation"
		xmppvoxVersion = "1.1"
	)
	var (
		dosvoxInfo = map[string]string{
			"root":    "C:\\winvox",
			"version": "4.0 BETA",
			"email":   "fulano@servidor.com.br",
		}
		machineInfo = map[string]string{
			"system":    "Windows",
			"node":      "network-name",
			"release":   "XP",
			"version":   "5.1.2600",
			"csd":       "SP2",
			"ptype":     "Multiprocessor Free",
			"machine":   "x86",
			"processor": "x86 Family 6 Model 23 Stepping 10, GenuineIntel",
		}
	)
	r, err := s.newInstallation(machineId, xmppvoxVersion, dosvoxInfo, machineInfo)
	c.Assert(err, IsNil)
	c.Check(r.StatusCode, Equals, http.StatusOK)
	installation := &Installation{}
	err = db.C("installations").FindId(machineId).One(installation)
	c.Assert(err, IsNil)
	c.Check(installation.CreatedAt.IsZero(), Equals, false)
	c.Check(installation.XMPPVOXVersion, Equals, xmppvoxVersion)
	c.Check(installation.DosvoxInfo, DeepEquals, dosvoxInfo)
	c.Check(installation.MachineInfo, DeepEquals, machineInfo)
}

func (s *WebAPISuite) TestNewInstallationDuplicateMachineId(c *C) {
	const (
		machineId      = "0e5ab64c-1b24-4917-new-installation-dup"
		xmppvoxVersion = "1.1"
	)
	var (
		dosvoxInfo = map[string]string{
			"root":    "C:\\winvox",
			"version": "4.0 BETA",
			"email":   "fulano@servidor.com.br",
		}
		machineInfo = map[string]string{
			"system":    "Windows",
			"node":      "network-name",
			"release":   "XP",
			"version":   "5.1.2600",
			"csd":       "SP2",
			"ptype":     "Multiprocessor Free",
			"machine":   "x86",
			"processor": "x86 Family 6 Model 23 Stepping 10, GenuineIntel",
		}
	)
	r, err := s.newInstallation(machineId, xmppvoxVersion, dosvoxInfo, machineInfo)
	c.Assert(err, IsNil)
	c.Check(r.StatusCode, Equals, http.StatusOK)
	// try to install again with the same info
	r, err = s.newInstallation(machineId, xmppvoxVersion, dosvoxInfo, machineInfo)
	c.Assert(err, IsNil)
	c.Check(r.StatusCode, Equals, http.StatusBadRequest)
}

func (s *WebAPISuite) TestNewInstallationMissingFields(c *C) {
	const (
		machineId      = "0e5ab64c-1b24-4917-new-installation-missing"
		xmppvoxVersion = "1.1"
	)
	var (
		dosvoxInfo = map[string]string{
			"root":    "C:\\winvox",
			"version": "4.0 BETA",
			"email":   "fulano@servidor.com.br",
		}
		machineInfo = map[string]string{
			"system":    "Windows",
			"node":      "network-name",
			"release":   "XP",
			"version":   "5.1.2600",
			"csd":       "SP2",
			"ptype":     "Multiprocessor Free",
			"machine":   "x86",
			"processor": "x86 Family 6 Model 23 Stepping 10, GenuineIntel",
		}
	)
	countBefore, err := db.C("installations").Find(nil).Count()
	c.Assert(err, IsNil)
	type TestCase struct {
		MachineId, XMPPVOXVersion string
		DosvoxInfo, MachineInfo   map[string]string
	}
	for _, tc := range []TestCase{
		TestCase{"", xmppvoxVersion, dosvoxInfo, machineInfo},
		TestCase{machineId, "", dosvoxInfo, machineInfo},
		TestCase{"", "", nil, nil},
	} {
		r, err := s.newInstallation(tc.MachineId, tc.XMPPVOXVersion, tc.DosvoxInfo, tc.MachineInfo)
		c.Assert(err, IsNil)
		c.Check(r.StatusCode, Equals, http.StatusBadRequest)
	}
	countAfter, err := db.C("installations").Find(nil).Count()
	c.Assert(err, IsNil)
	c.Check(countAfter, Equals, countBefore)
}

// New Session tests

func (s *WebAPISuite) TestNewSession(c *C) {
	const (
		jid            = "testuser@server.org"
		machineId      = "00:26:cc:18:be:14"
		xmppvoxVersion = "1.0"
	)
	r, err := s.newSession(jid, machineId, xmppvoxVersion)
	c.Assert(err, IsNil)
	c.Check(r.StatusCode, Equals, http.StatusOK)
	idHex := strings.TrimSpace(r.Body)
	c.Assert(bson.IsObjectIdHex(idHex), Equals, true)
	id := bson.ObjectIdHex(idHex)
	session := &Session{}
	err = db.C("sessions").FindId(id).One(session)
	c.Assert(err, IsNil)
	c.Check(session.CreatedAt.IsZero(), Equals, false)
	c.Check(session.ClosedAt.IsZero(), Equals, true)
	c.Check(session.LastPing.IsZero(), Equals, true)
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
		r, err := s.newSession(tc.JID, tc.MachineId, tc.XMPPVOXVersion)
		c.Assert(err, IsNil)
		c.Check(r.StatusCode, Equals, http.StatusBadRequest)
	}
	countAfter, err := db.C("sessions").Find(nil).Count()
	c.Assert(err, IsNil)
	c.Check(countAfter, Equals, countBefore)
}

func (s *WebAPISuite) TestNewSessionExtraFields(c *C) {
	const (
		jid               = "testuser@server.org"
		machineId         = "00:26:cc:18:be:14"
		xmppvoxVersion    = "1.0"
		extraInvalidField = "this is invalid"
	)
	r, err := s.apiPostCall("/1/session/new", map[string]string{
		"jid":                 jid,
		"machine_id":          machineId,
		"xmppvox_version":     xmppvoxVersion,
		"extra_invalid_field": extraInvalidField,
	})
	c.Assert(err, IsNil)
	c.Check(r.StatusCode, Equals, http.StatusBadRequest)
}

// Close Session tests

func (s *WebAPISuite) TestCloseSession(c *C) {
	nr, err := s.newSession("testuser@server.org", "00:26:cc:18:be:14", "1.0")
	c.Assert(err, IsNil)
	id := bson.ObjectIdHex(strings.TrimSpace(nr.Body))
	cr, err := s.closeSession(id, "00:26:cc:18:be:14")
	c.Assert(err, IsNil)
	c.Check(cr.StatusCode, Equals, http.StatusOK)
	c.Check(cr.Body, Equals, nr.Body)
	session := &Session{}
	err = db.C("sessions").FindId(id).One(session)
	c.Assert(err, IsNil)
	c.Check(session.ClosedAt.IsZero(), Equals, false)
}

func (s *WebAPISuite) TestCloseSessionExtraFields(c *C) {
	nr, err := s.newSession("testuser@server.org", "00:26:cc:18:be:14", "1.0")
	c.Assert(err, IsNil)
	id := bson.ObjectIdHex(strings.TrimSpace(nr.Body))
	cr, err := s.apiPostCall("/1/session/close", map[string]string{
		"session_id":          id.Hex(),
		"machine_id":          "00:26:cc:18:be:14",
		"extra_invalid_field": "this is invalid",
	})
	c.Assert(err, IsNil)
	c.Check(cr.StatusCode, Equals, http.StatusBadRequest)
}

func (s *WebAPISuite) TestCloseSessionInexistent(c *C) {
	r, err := s.closeSession(bson.NewObjectId(), "00:26:cc:18:be:14")
	c.Assert(err, IsNil)
	c.Check(r.StatusCode, Equals, http.StatusBadRequest)
}

func (s *WebAPISuite) TestCloseSessionAlreadyClosed(c *C) {
	nr, err := s.newSession("testuser@server.org", "00:26:cc:18:be:14", "1.0")
	c.Assert(err, IsNil)
	id := bson.ObjectIdHex(strings.TrimSpace(nr.Body))
	cr, err := s.closeSession(id, "00:26:cc:18:be:14")
	c.Assert(err, IsNil)
	c.Check(cr.StatusCode, Equals, http.StatusOK)
	session := &Session{}
	err = db.C("sessions").FindId(id).One(session)
	c.Assert(err, IsNil)
	closedAtBefore := session.ClosedAt
	// Close the same session again
	cr, err = s.closeSession(id, "00:26:cc:18:be:14")
	c.Assert(err, IsNil)
	c.Check(cr.StatusCode, Equals, http.StatusBadRequest)
	// Check session.ClosedAt value
	err = db.C("sessions").FindId(id).One(session)
	c.Assert(err, IsNil)
	closedAtAfter := session.ClosedAt
	c.Check(closedAtAfter, Equals, closedAtBefore)
}

func (s *WebAPISuite) TestCannotCloseSomebodyElsesSession(c *C) {
	nr, err := s.newSession("testuser@server.org", "00:26:cc:18:be:14", "1.0")
	c.Assert(err, IsNil)
	id := bson.ObjectIdHex(strings.TrimSpace(nr.Body))
	cr, err := s.closeSession(id, "ANOTHER_MACHINE_ID")
	c.Assert(err, IsNil)
	c.Check(cr.StatusCode, Equals, http.StatusBadRequest)
}

// Ping Session tests

func (s *WebAPISuite) TestPingSession(c *C) {
	nr, err := s.newSession("testuser@server.org", "00:26:cc:18:be:14", "1.0")
	c.Assert(err, IsNil)
	id := bson.ObjectIdHex(strings.TrimSpace(nr.Body))
	cr, err := s.pingSession(id, "00:26:cc:18:be:14")
	c.Assert(err, IsNil)
	c.Check(cr.StatusCode, Equals, http.StatusOK)
	c.Check(cr.Body, Equals, nr.Body)
	session := &Session{}
	err = db.C("sessions").FindId(id).One(session)
	c.Assert(err, IsNil)
	c.Check(session.LastPing.IsZero(), Equals, false)
}

func (s *WebAPISuite) TestPingSessionExtraFields(c *C) {
	nr, err := s.newSession("testuser@server.org", "00:26:cc:18:be:14", "1.0")
	c.Assert(err, IsNil)
	id := bson.ObjectIdHex(strings.TrimSpace(nr.Body))
	cr, err := s.apiPostCall("/1/session/ping", map[string]string{
		"session_id":          id.Hex(),
		"machine_id":          "00:26:cc:18:be:14",
		"extra_invalid_field": "this is invalid",
	})
	c.Assert(err, IsNil)
	c.Check(cr.StatusCode, Equals, http.StatusBadRequest)
}

func (s *WebAPISuite) TestPingSessionInexistent(c *C) {
	r, err := s.pingSession(bson.NewObjectId(), "00:26:cc:18:be:14")
	c.Assert(err, IsNil)
	c.Check(r.StatusCode, Equals, http.StatusBadRequest)
}

func (s *WebAPISuite) TestPingSessionAlreadyClosed(c *C) {
	nr, err := s.newSession("testuser@server.org", "00:26:cc:18:be:14", "1.0")
	c.Assert(err, IsNil)
	id := bson.ObjectIdHex(strings.TrimSpace(nr.Body))
	cr, err := s.closeSession(id, "00:26:cc:18:be:14")
	c.Assert(err, IsNil)
	c.Check(cr.StatusCode, Equals, http.StatusOK)
	session := &Session{}
	err = db.C("sessions").FindId(id).One(session)
	c.Assert(err, IsNil)
	lastPingBefore := session.LastPing
	// PING closed session
	cr, err = s.pingSession(id, "00:26:cc:18:be:14")
	c.Assert(err, IsNil)
	c.Check(cr.StatusCode, Equals, http.StatusBadRequest)
	// Check session.LastPing value
	err = db.C("sessions").FindId(id).One(session)
	c.Assert(err, IsNil)
	lastPingAfter := session.LastPing
	c.Check(lastPingAfter, Equals, lastPingBefore)
}

func (s *WebAPISuite) TestPingSessionTwice(c *C) {
	nr, err := s.newSession("testuser@server.org", "00:26:cc:18:be:14", "1.0")
	c.Assert(err, IsNil)
	id := bson.ObjectIdHex(strings.TrimSpace(nr.Body))
	// First PING
	cr, err := s.pingSession(id, "00:26:cc:18:be:14")
	c.Assert(err, IsNil)
	c.Check(cr.StatusCode, Equals, http.StatusOK)
	session := &Session{}
	err = db.C("sessions").FindId(id).One(session)
	c.Assert(err, IsNil)
	lastPingBefore := session.LastPing
	middleTime := bson.Now()
	// Second PING
	cr, err = s.pingSession(id, "00:26:cc:18:be:14")
	c.Assert(err, IsNil)
	c.Check(cr.StatusCode, Equals, http.StatusOK)
	// Check session.LastPing value
	err = db.C("sessions").FindId(id).One(session)
	c.Assert(err, IsNil)
	lastPingAfter := session.LastPing
	// Check that lastPingBefore <= middleTime <= lastPingAfter
	c.Check(lastPingBefore.After(middleTime) || middleTime.After(lastPingAfter), Equals, false)
}

func (s *WebAPISuite) TestCannotPingSomebodyElsesSession(c *C) {
	nr, err := s.newSession("testuser@server.org", "00:26:cc:18:be:14", "1.0")
	c.Assert(err, IsNil)
	id := bson.ObjectIdHex(strings.TrimSpace(nr.Body))
	cr, err := s.pingSession(id, "ANOTHER_MACHINE_ID")
	c.Assert(err, IsNil)
	c.Check(cr.StatusCode, Equals, http.StatusBadRequest)
}
