Elephant Tracker
================

Elephant Tracker is a usage tracker for [XMPPVOX](https://github.com/rhcarvalho/xmppvox).


Installing
----------

    go get github.com/rhcarvalho/elephant-tracker


Running
-------

    elephant-tracker --config /path/to/config.json


Configuration example
---------------------

config.json:

```json
{
  "http": {
    "host": "localhost",
    "port": 424242
  },
  "mongo": {
    "url": "user:password@localhost:142857",
    "db": "xmppvox"
  }
}```

