/*

API v1 documentation

  HTTP_METHOD URL (params, ...)

  POST /installation/new (machine_id, xmppvox_version, dosvox_info, machine_info)

Registers a new XMPPVOX installation. All params must be non-empty strings.
dosvox_info and machine_info can either be null or contain a JSON-encoded mapping
of strings to strings.
Returns the machine_id.

  POST /session/new (jid, machine_id, xmppvox_version)

Registers a new XMPPVOX session. All params must be non-empty.
Returns the ID of the session in the first line of the response
and might return a message in the next lines.

  POST /session/close (session_id, machine_id)

Closes an existing XMPPVOX session.
The machine_id is required as a minimal security feature
to prevent an attacker from closing arbitrary sessions.
Returns the ID of the session.

  POST /session/ping (session_id, machine_id)

Pings an existing open XMPPVOX session.
Returns the ID of the session.

Note: All responses have one of 200, 400 or 500 status code.

*/
package main
