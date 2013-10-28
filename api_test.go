package main

import (
	"encoding/json"
	"errors"
	"labix.org/v2/mgo"
	"labix.org/v2/mgo/bson"
	. "launchpad.net/gocheck"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
	"time"
)

// Hook up gocheck into the "go test" runner.
func Test(t *testing.T) { TestingT(t) }

type WebAPISuite struct {
	Store Storage
}

var _ = Suite(&WebAPISuite{})

type TestStore struct {
	Installations map[string]*Installation
	Sessions      map[bson.ObjectId]*Session
}

func (s *WebAPISuite) SetUpTest(c *C) {
	s.Store = &TestStore{
		make(map[string]*Installation),
		make(map[bson.ObjectId]*Session),
	}
}

type Response struct {
	Body       string
	StatusCode int
}

func (ts *TestStore) InsertInstallation(i *Installation) error {
	if _, ok := ts.Installations[i.MachineId]; ok {
		// the incantation below makes mgo.IsDup(err) == true
		return &mgo.QueryError{Code: 11000}
	}
	ts.Installations[i.MachineId] = i
	return nil
}
func (ts *TestStore) InsertSession(s *Session) error {
	if _, ok := ts.Sessions[s.Id]; ok {
		return errors.New("duplicate")
	}
	ts.Sessions[s.Id] = s
	return nil
}
func (ts *TestStore) CloseSession(s *Session) error {
	if tss, ok := ts.Sessions[s.Id]; ok {
		if tss.MachineId == s.MachineId && tss.ClosedAt.Equal(time.Time{}) {
			tss.ClosedAt = bson.Now()
			return nil
		}
	}
	return mgo.ErrNotFound
}
func (ts *TestStore) PingSession(s *Session) error {
	if tss, ok := ts.Sessions[s.Id]; ok {
		if tss.MachineId == s.MachineId && tss.ClosedAt.Equal(time.Time{}) {
			tss.LastPing = bson.Now()
			return nil
		}
	}
	return mgo.ErrNotFound
}

func (s *WebAPISuite) handlePost(h contextualHandlerFunc, data map[string]string) *Response {
	postData := url.Values{}
	for key, value := range data {
		postData.Set(key, value)
	}
	req, err := http.NewRequest("POST", "Dummy URL", strings.NewReader(postData.Encode()))
	if err != nil {
		panic(err)
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	w := httptest.NewRecorder()
	h(w, req, &Context{s.Store})
	return &Response{
		Body:       w.Body.String(),
		StatusCode: w.Code,
	}
}

func (s *WebAPISuite) newInstallation(machineId, xmppvoxVersion string, dosvoxInfo, machineInfo map[string]string) *Response {
	m := func(v interface{}) string {
		b, _ := json.Marshal(v)
		return string(b)
	}
	return s.handlePost(NewInstallationHandler, map[string]string{
		"machine_id":      machineId,
		"xmppvox_version": xmppvoxVersion,
		"dosvox_info":     m(dosvoxInfo),
		"machine_info":    m(machineInfo),
	})

}

func (s *WebAPISuite) newSession(jid, machineId, xmppvoxVersion string) *Response {
	return s.handlePost(NewSessionHandler, map[string]string{
		"jid":             jid,
		"machine_id":      machineId,
		"xmppvox_version": xmppvoxVersion,
	})
}

func (s *WebAPISuite) closeSession(sessionId bson.ObjectId, machineId string) *Response {
	return s.handlePost(CloseSessionHandler, map[string]string{
		"session_id": sessionId.Hex(),
		"machine_id": machineId,
	})
}

func (s *WebAPISuite) pingSession(sessionId bson.ObjectId, machineId string) *Response {
	return s.handlePost(PingSessionHandler, map[string]string{
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
	r := s.newInstallation(machineId, xmppvoxVersion, dosvoxInfo, machineInfo)
	c.Check(r.StatusCode, Equals, http.StatusOK)
	installation := s.Store.(*TestStore).Installations[machineId]
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
	r := s.newInstallation(machineId, xmppvoxVersion, dosvoxInfo, machineInfo)
	c.Check(r.StatusCode, Equals, http.StatusOK)
	// try to install again with the same info
	r = s.newInstallation(machineId, xmppvoxVersion, dosvoxInfo, machineInfo)
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
	countBefore := len(s.Store.(*TestStore).Installations)
	type TestCase struct {
		MachineId, XMPPVOXVersion string
		DosvoxInfo, MachineInfo   map[string]string
	}
	for _, tc := range []TestCase{
		TestCase{"", xmppvoxVersion, dosvoxInfo, machineInfo},
		TestCase{machineId, "", dosvoxInfo, machineInfo},
		TestCase{"", "", nil, nil},
	} {
		r := s.newInstallation(tc.MachineId, tc.XMPPVOXVersion, tc.DosvoxInfo, tc.MachineInfo)
		c.Check(r.StatusCode, Equals, http.StatusBadRequest)
	}
	countAfter := len(s.Store.(*TestStore).Installations)
	c.Check(countAfter, Equals, countBefore)
}

// New Session tests

func (s *WebAPISuite) TestNewSession(c *C) {
	const (
		jid            = "testuser@server.org"
		machineId      = "00:26:cc:18:be:14"
		xmppvoxVersion = "1.0"
	)
	r := s.newSession(jid, machineId, xmppvoxVersion)
	c.Check(r.StatusCode, Equals, http.StatusOK)
	idHex := strings.TrimSpace(r.Body)
	c.Assert(bson.IsObjectIdHex(idHex), Equals, true)
	id := bson.ObjectIdHex(idHex)
	session := s.Store.(*TestStore).Sessions[id]
	c.Check(session.CreatedAt.IsZero(), Equals, false)
	c.Check(session.ClosedAt.IsZero(), Equals, true)
	c.Check(session.LastPing.IsZero(), Equals, true)
	c.Check(session.JID, Equals, jid)
	c.Check(session.MachineId, Equals, machineId)
	c.Check(session.XMPPVOXVersion, Equals, xmppvoxVersion)
	c.Check(session.Request, NotNil)
}

func (s *WebAPISuite) TestNewSessionMissingFields(c *C) {
	countBefore := len(s.Store.(*TestStore).Sessions)
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
		r := s.newSession(tc.JID, tc.MachineId, tc.XMPPVOXVersion)
		c.Check(r.StatusCode, Equals, http.StatusBadRequest)
	}
	countAfter := len(s.Store.(*TestStore).Sessions)
	c.Check(countAfter, Equals, countBefore)
}

func (s *WebAPISuite) TestNewSessionExtraFields(c *C) {
	const (
		jid               = "testuser@server.org"
		machineId         = "00:26:cc:18:be:14"
		xmppvoxVersion    = "1.0"
		extraInvalidField = "this is invalid"
	)
	r := s.handlePost(NewSessionHandler, map[string]string{
		"jid":                 jid,
		"machine_id":          machineId,
		"xmppvox_version":     xmppvoxVersion,
		"extra_invalid_field": extraInvalidField,
	})
	c.Check(r.StatusCode, Equals, http.StatusBadRequest)
}

// Close Session tests

func (s *WebAPISuite) TestCloseSession(c *C) {
	nr := s.newSession("testuser@server.org", "00:26:cc:18:be:14", "1.0")
	id := bson.ObjectIdHex(strings.TrimSpace(nr.Body))
	cr := s.closeSession(id, "00:26:cc:18:be:14")
	c.Check(cr.StatusCode, Equals, http.StatusOK)
	c.Check(cr.Body, Equals, nr.Body)
	session := s.Store.(*TestStore).Sessions[id]
	c.Check(session.ClosedAt.IsZero(), Equals, false)
}

func (s *WebAPISuite) TestCloseSessionExtraFields(c *C) {
	nr := s.newSession("testuser@server.org", "00:26:cc:18:be:14", "1.0")
	id := bson.ObjectIdHex(strings.TrimSpace(nr.Body))
	cr := s.handlePost(CloseSessionHandler, map[string]string{
		"session_id":          id.Hex(),
		"machine_id":          "00:26:cc:18:be:14",
		"extra_invalid_field": "this is invalid",
	})
	c.Check(cr.StatusCode, Equals, http.StatusBadRequest)
}

func (s *WebAPISuite) TestCloseSessionInexistent(c *C) {
	r := s.closeSession(bson.NewObjectId(), "00:26:cc:18:be:14")
	c.Check(r.StatusCode, Equals, http.StatusBadRequest)
}

func (s *WebAPISuite) TestCloseSessionAlreadyClosed(c *C) {
	nr := s.newSession("testuser@server.org", "00:26:cc:18:be:14", "1.0")
	id := bson.ObjectIdHex(strings.TrimSpace(nr.Body))
	cr := s.closeSession(id, "00:26:cc:18:be:14")
	c.Check(cr.StatusCode, Equals, http.StatusOK)
	session := s.Store.(*TestStore).Sessions[id]
	closedAtBefore := session.ClosedAt
	// Close the same session again
	cr = s.closeSession(id, "00:26:cc:18:be:14")
	c.Check(cr.StatusCode, Equals, http.StatusBadRequest)
	// Check session.ClosedAt value
	session = s.Store.(*TestStore).Sessions[id]
	closedAtAfter := session.ClosedAt
	c.Check(closedAtAfter, Equals, closedAtBefore)
}

func (s *WebAPISuite) TestCannotCloseSomebodyElsesSession(c *C) {
	nr := s.newSession("testuser@server.org", "00:26:cc:18:be:14", "1.0")
	id := bson.ObjectIdHex(strings.TrimSpace(nr.Body))
	cr := s.closeSession(id, "ANOTHER_MACHINE_ID")
	c.Check(cr.StatusCode, Equals, http.StatusBadRequest)
}

// Ping Session tests

func (s *WebAPISuite) TestPingSession(c *C) {
	nr := s.newSession("testuser@server.org", "00:26:cc:18:be:14", "1.0")
	id := bson.ObjectIdHex(strings.TrimSpace(nr.Body))
	cr := s.pingSession(id, "00:26:cc:18:be:14")
	c.Check(cr.StatusCode, Equals, http.StatusOK)
	c.Check(cr.Body, Equals, nr.Body)
	session := s.Store.(*TestStore).Sessions[id]
	c.Check(session.LastPing.IsZero(), Equals, false)
}

func (s *WebAPISuite) TestPingSessionExtraFields(c *C) {
	nr := s.newSession("testuser@server.org", "00:26:cc:18:be:14", "1.0")
	id := bson.ObjectIdHex(strings.TrimSpace(nr.Body))
	cr := s.handlePost(PingSessionHandler, map[string]string{
		"session_id":          id.Hex(),
		"machine_id":          "00:26:cc:18:be:14",
		"extra_invalid_field": "this is invalid",
	})
	c.Check(cr.StatusCode, Equals, http.StatusBadRequest)
}

func (s *WebAPISuite) TestPingSessionInexistent(c *C) {
	r := s.pingSession(bson.NewObjectId(), "00:26:cc:18:be:14")
	c.Check(r.StatusCode, Equals, http.StatusBadRequest)
}

func (s *WebAPISuite) TestPingSessionAlreadyClosed(c *C) {
	nr := s.newSession("testuser@server.org", "00:26:cc:18:be:14", "1.0")
	id := bson.ObjectIdHex(strings.TrimSpace(nr.Body))
	cr := s.closeSession(id, "00:26:cc:18:be:14")
	c.Check(cr.StatusCode, Equals, http.StatusOK)
	session := s.Store.(*TestStore).Sessions[id]
	lastPingBefore := session.LastPing
	// PING closed session
	cr = s.pingSession(id, "00:26:cc:18:be:14")
	c.Check(cr.StatusCode, Equals, http.StatusBadRequest)
	// Check session.LastPing value
	session = s.Store.(*TestStore).Sessions[id]
	lastPingAfter := session.LastPing
	c.Check(lastPingAfter, Equals, lastPingBefore)
}

func (s *WebAPISuite) TestPingSessionTwice(c *C) {
	nr := s.newSession("testuser@server.org", "00:26:cc:18:be:14", "1.0")
	id := bson.ObjectIdHex(strings.TrimSpace(nr.Body))
	// First PING
	cr := s.pingSession(id, "00:26:cc:18:be:14")
	c.Check(cr.StatusCode, Equals, http.StatusOK)
	session := s.Store.(*TestStore).Sessions[id]
	lastPingBefore := session.LastPing
	middleTime := bson.Now()
	// Second PING
	cr = s.pingSession(id, "00:26:cc:18:be:14")
	c.Check(cr.StatusCode, Equals, http.StatusOK)
	// Check session.LastPing value
	session = s.Store.(*TestStore).Sessions[id]
	lastPingAfter := session.LastPing
	// Check that lastPingBefore <= middleTime <= lastPingAfter
	c.Check(lastPingBefore.After(middleTime) || middleTime.After(lastPingAfter), Equals, false)
}

func (s *WebAPISuite) TestCannotPingSomebodyElsesSession(c *C) {
	nr := s.newSession("testuser@server.org", "00:26:cc:18:be:14", "1.0")
	id := bson.ObjectIdHex(strings.TrimSpace(nr.Body))
	cr := s.pingSession(id, "ANOTHER_MACHINE_ID")
	c.Check(cr.StatusCode, Equals, http.StatusBadRequest)
}
