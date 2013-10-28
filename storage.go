package main

import (
	"labix.org/v2/mgo"
	"labix.org/v2/mgo/bson"
	"net/http"
	"net/url"
	"time"
)

// Installation stores information about a XMPPVOX installation.
type Installation struct {
	MachineId      string            `bson:"_id"`
	XMPPVOXVersion string            `bson:"xmppvox_ver"`
	DosvoxInfo     map[string]string `bson:"dosvox_info"`
	MachineInfo    map[string]string `bson:"machine_info"`
	CreatedAt      time.Time         `bson:"created_at"`
}

// Session stores information about a XMPPVOX session.
type Session struct {
	Id             bson.ObjectId `bson:"_id"`
	CreatedAt      time.Time     `bson:"created_at"`
	ClosedAt       time.Time     `bson:"closed_at"`
	LastPing       time.Time     `bson:"last_ping"`
	JID            string        `bson:"jid"`
	MachineId      string        `bson:"machine_id"`
	XMPPVOXVersion string        `bson:"xmppvox_ver"`
	Request        *HttpRequest  `bson:"req"`
}

// HttpRequest is a subset of http.Request.
type HttpRequest struct {
	Method string
	URL    *url.URL
	Header http.Header
	//Body io.ReadCloser
	//ContentLength int64
	//TransferEncoding []string
	//Close bool
	Host string
	Form url.Values
	//PostForm url.Values
	//MultipartForm *multipart.Form
	//Trailer Header
	RemoteAddr string
	//RequestURI string
	//TLS *tls.ConnectionState
}

func NewInstallation(machineId, xmppvoxVersion string, dosvoxInfo, machineInfo map[string]string) *Installation {
	return &Installation{
		MachineId:      machineId,
		XMPPVOXVersion: xmppvoxVersion,
		DosvoxInfo:     dosvoxInfo,
		MachineInfo:    machineInfo,
		CreatedAt:      bson.Now(),
	}
}

func InsertInstallation(i *Installation) error {
	coll := db.C("installations")
	return coll.Insert(i)
}

func NewSession(jid, machineId, xmppvoxVersion string, r *HttpRequest) *Session {
	return &Session{
		Id:             bson.NewObjectId(),
		CreatedAt:      bson.Now(),
		JID:            jid,
		MachineId:      machineId,
		XMPPVOXVersion: xmppvoxVersion,
		Request:        r,
	}
}

func InsertSession(s *Session) error {
	coll := db.C("sessions")
	return coll.Insert(s)
}

func CloseSession(sessionId bson.ObjectId, machineId string) (info *mgo.ChangeInfo, err error) {
	coll := db.C("sessions")
	session := &Session{}
	updateClosedTime := mgo.Change{
		Update:    bson.M{"$set": bson.M{"closed_at": bson.Now()}},
		ReturnNew: true,
	}
	return coll.Find(bson.M{"_id": sessionId, "machine_id": machineId, "closed_at": time.Time{}}).
		Apply(updateClosedTime, &session)
}

func PingSession(sessionId bson.ObjectId, machineId string) (info *mgo.ChangeInfo, err error) {
	coll := db.C("sessions")
	session := &Session{}
	updateLastPing := mgo.Change{
		Update:    bson.M{"$set": bson.M{"last_ping": bson.Now()}},
		ReturnNew: true,
	}
	return coll.Find(bson.M{"_id": sessionId, "machine_id": machineId, "closed_at": time.Time{}}).
		Apply(updateLastPing, &session)
}
