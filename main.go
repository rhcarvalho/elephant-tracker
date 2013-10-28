package main

import (
	"flag"
	"fmt"
	"labix.org/v2/mgo"
	"log"
	"net/http"
	"time"
)

var configPath = flag.String("config", "config.json", "path to a configuration file in JSON format")
var db *mgo.Database

func main() {
	flag.Parse()
	config, err := ConfigOpen(*configPath)
	if err != nil {
		log.Fatalln(err)
	}

	// Set session timeout to fail early and avoid long response times.
	session, err := mgo.DialWithTimeout(config.Mongo.URL, 5*time.Second)
	if err != nil {
		log.Fatalln("[MongoDB]", err)
	}
	defer session.Close()

	db = session.DB(config.Mongo.DB)

	addr := fmt.Sprintf("%s:%d", config.Http.Host, config.Http.Port)
	log.Printf("serving at %s\n", addr)
	err = http.ListenAndServe(addr, APIHandler())
	if err != nil {
		log.Fatal(err)
	}
}
