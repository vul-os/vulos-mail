#!/usr/bin/env python3
"""Comprehensive end-to-end driver for the Dockerized vulos-mail ecosystem.

Exercises every protocol + feature path on a single instance and across two
servers (a.test <-> b.test) with real DNS-based SPF/DKIM/DMARC, plus a TLS
instance (c.test). Exits non-zero if any check fails. RESTART_CHECK=1 runs only a
post-restart persistence probe.
"""
import base64
import imaplib
import json
import os
import smtplib
import ssl
import sys
import threading
import time
import urllib.error
import urllib.request

A, B, T = "mta-a", "mta-b", "mta-tls"
SUBMIT, IMAP, JMAP, MX, METRICS = 587, 143, 8080, 25, 9090
ALICE, BOB, CAROL, PW = "alice@a.test", "bob@b.test", "carol@c.test", "pw"

NOVERIFY = ssl.create_default_context()
NOVERIFY.check_hostname = False
NOVERIFY.verify_mode = ssl.CERT_NONE

results = []


def check(name, ok, detail=""):
    results.append((name, bool(ok)))
    print(f"  [{'PASS' if ok else 'FAIL'}] {name}" + (f" — {detail}" if detail else ""), flush=True)


def phase(title):
    print(f"== {title} ==", flush=True)


def run(name, fn):
    """Run a check fn() -> (ok, detail) | bool, turning exceptions into FAIL."""
    try:
        r = fn()
        if isinstance(r, tuple):
            check(name, r[0], r[1])
        else:
            check(name, r)
    except Exception as e:
        check(name, False, f"{type(e).__name__}: {e}")


def basic(user, pw):
    return "Basic " + base64.b64encode(f"{user}:{pw}".encode()).decode()


def http(host, path, user=None, pw=None, method="GET", body=None, tls=False, port=JMAP):
    scheme = "https" if tls else "http"
    url = f"{scheme}://{host}:{port}{path}"
    req = urllib.request.Request(url, data=body.encode() if body else None, method=method)
    if user:
        req.add_header("Authorization", basic(user, pw))
    if body:
        req.add_header("Content-Type", "application/json")
    ctx = NOVERIFY if tls else None
    try:
        with urllib.request.urlopen(req, timeout=20, context=ctx) as r:
            return r.status, r.read().decode()
    except urllib.error.HTTPError as e:
        return e.code, e.read().decode()


def jmap(host, user, pw, calls, using=("core", "mail"), tls=False):
    urns = [f"urn:ietf:params:jmap:{u}" for u in using]
    payload = json.dumps({"using": urns, "methodCalls": calls})
    st, txt = http(host, "/jmap/api", user, pw, "POST", payload, tls=tls)
    if st != 200:
        raise RuntimeError(f"JMAP HTTP {st}: {txt[:200]}")
    return json.loads(txt)["methodResponses"]


def jcall(host, user, pw, method, args, using=("core", "mail")):
    return jmap(host, user, pw, [[method, args, "0"]], using)[0][1]


def deliver_mx(host, mail_from, rcpts, raw, starttls=False):
    s = smtplib.SMTP(host, MX, timeout=20)
    try:
        if starttls:
            s.starttls(context=NOVERIFY)
        s.sendmail(mail_from, rcpts, raw)
    finally:
        try:
            s.quit()
        except Exception:
            pass


def submit(host, user, pw, mail_from, rcpts, raw, starttls=False):
    s = smtplib.SMTP(host, SUBMIT, timeout=20)
    try:
        if starttls:
            s.starttls(context=NOVERIFY)
        s.login(user, pw)
        s.sendmail(mail_from, rcpts, raw)
    finally:
        try:
            s.quit()
        except Exception:
            pass


def wait_ready(host, user, timeout=90, tls=False):
    deadline = time.time() + timeout
    while time.time() < deadline:
        try:
            st, _ = http(host, "/jmap/session", user, PW, tls=tls)
            if st == 200:
                return True
        except Exception:
            pass
        time.sleep(1)
    return False


def query_ids(host, user, mailbox="inbox"):
    r = jcall(host, user, PW, "Email/query", {"accountId": user, "filter": {"inMailbox": mailbox}})
    return r.get("ids", [])


def find_by_subject(host, user, subject, timeout=70):
    """Poll a JMAP inbox until a message with the given subject arrives; return its id."""
    deadline = time.time() + timeout
    while time.time() < deadline:
        ids = query_ids(host, user)
        if ids:
            g = jcall(host, user, PW, "Email/get", {"accountId": user, "ids": ids, "properties": ["subject"]})
            for m in g.get("list", []):
                if m.get("subject") == subject:
                    return m["id"]
        time.sleep(2)
    return None


def imap_find_raw(host, user, subject, timeout=70):
    """Poll an IMAP inbox for a message with subject; return its raw RFC822."""
    deadline = time.time() + timeout
    while time.time() < deadline:
        try:
            c = imaplib.IMAP4(host, IMAP)
            c.login(user, PW)
            c.select("INBOX")
            _, data = c.search(None, "ALL")
            for i in (data[0].split() if data and data[0] else []):
                _, fd = c.fetch(i, "(BODY[])")
                raw = fd[0][1].decode(errors="replace") if fd and fd[0] else ""
                if subject in raw:
                    c.logout()
                    return raw
            c.logout()
        except Exception:
            pass
        time.sleep(2)
    return ""


# ------------------------------------------------------------------ restart probe
def restart_probe():
    phase("post-restart persistence (mta-a)")
    run("mta-a ready after restart", lambda: wait_ready(A, ALICE))
    run("alice inbox survived restart", lambda: (len(query_ids(A, ALICE)) >= 1, f"{len(query_ids(A, ALICE))} msgs"))
    summary()


def summary():
    passed = sum(1 for _, ok in results if ok)
    total = len(results)
    print(f"\n=== {passed}/{total} checks passed ===", flush=True)
    sys.exit(0 if passed == total else 1)


# ------------------------------------------------------------------ full suite
def main():
    if os.environ.get("RESTART_CHECK"):
        restart_probe()
        return

    phase("readiness")
    run("mta-a ready", lambda: wait_ready(A, ALICE))
    run("mta-b ready", lambda: wait_ready(B, BOB))
    run("mta-tls ready (HTTPS)", lambda: wait_ready(T, CAROL, tls=True))

    phase("MX receive / IMAP / JMAP (mta-a)")
    deliver_mx(A, "ext@out.example", [ALICE],
               f"From: Outsider <ext@out.example>\r\nTo: {ALICE}\r\nSubject: hello a\r\n\r\nbody one\r\n")
    run("inbound MX -> inbox", lambda: (len(query_ids(A, ALICE)) >= 1, f"{len(query_ids(A, ALICE))} msgs"))

    def imap_smoke():
        c = imaplib.IMAP4(A, IMAP)
        c.login(ALICE, PW)
        typ, _ = c.select("INBOX")
        _, d = c.search(None, "ALL")
        n = len(d[0].split()) if d and d[0] else 0
        c.logout()
        return typ == "OK" and n >= 1, f"{n} in INBOX"
    run("IMAP login+select+search", imap_smoke)

    def jmap_getset():
        ids = query_ids(A, ALICE)
        g = jcall(A, ALICE, PW, "Email/get", {"accountId": ALICE, "ids": ids[:1], "properties": ["subject"]})
        if not g["list"]:
            return False, "no message"
        s = jcall(A, ALICE, PW, "Email/set", {"accountId": ALICE, "update": {ids[0]: {"keywords/$seen": True}}})
        return ids[0] in (s.get("updated") or {}), "get+set ok"
    run("JMAP Email/get + set", jmap_getset)
    run("JMAP Identity/get", lambda: jcall(A, ALICE, PW, "Identity/get", {"accountId": ALICE}, using=("core", "mail", "submission"))["list"][0]["email"] == ALICE)

    phase("webmail APIs (mta-a)")
    run("contacts add+list", lambda: (http(A, "/api/webmail/contacts", ALICE, PW, "POST", json.dumps({"name": "Z", "email": "z@x.test"}))[0] == 200
                                      and "z@x.test" in http(A, "/api/webmail/contacts", ALICE, PW)[1]))
    run("calendar add+list", lambda: (http(A, "/api/webmail/calendar", ALICE, PW, "POST", json.dumps({"summary": "Standup", "start": "2026-06-22T09:00:00Z"}))[0] == 200
                                      and "Standup" in http(A, "/api/webmail/calendar", ALICE, PW)[1]))
    run("settings save+read", lambda: (http(A, "/api/webmail/settings", ALICE, PW, "POST", json.dumps({"signature": "Alice", "vacation": {"enabled": False, "subject": "", "body": ""}}))[0] == 200
                                      and "Alice" in http(A, "/api/webmail/settings", ALICE, PW)[1]))

    phase("DAV write round-trips (mta-a)")
    def caldav_put():
        ics = ("BEGIN:VCALENDAR\r\nVERSION:2.0\r\nPRODID:-//t//EN\r\nBEGIN:VEVENT\r\nUID:probe1\r\n"
               "DTSTAMP:20260622T090000Z\r\nDTSTART:20260622T100000Z\r\nSUMMARY:DAV Event\r\nEND:VEVENT\r\nEND:VCALENDAR\r\n")
        st = http(A, f"/dav/calendars/{ALICE}/probe1.ics", ALICE, PW, "PUT", ics)[0]
        seen = "DAV Event" in http(A, "/api/webmail/calendar", ALICE, PW)[1]
        return st in (200, 201, 204) and seen, f"PUT {st}, visible={seen}"
    run("CalDAV PUT -> webmail calendar", caldav_put)

    def carddav_put():
        vcf = "BEGIN:VCARD\r\nVERSION:4.0\r\nFN:DAV Person\r\nEMAIL:dav@x.test\r\nEND:VCARD\r\n"
        st = http(A, f"/dav/addressbooks/{ALICE}/probe1.vcf", ALICE, PW, "PUT", vcf)[0]
        seen = "DAV Person" in http(A, "/api/webmail/contacts", ALICE, PW)[1] or "dav@x.test" in http(A, "/api/webmail/contacts", ALICE, PW)[1]
        return st in (200, 201, 204) and seen, f"PUT {st}, visible={seen}"
    run("CardDAV PUT -> webmail contacts", carddav_put)

    phase("live push (SSE)")
    def sse_push():
        st, txt = http(A, "/api/webmail/pushtoken", ALICE, PW, "POST")
        tok = json.loads(txt)["token"]
        resp = urllib.request.urlopen(f"http://{A}:{JMAP}/api/webmail/changes?token={tok}", timeout=20)
        threading.Timer(1.5, lambda: deliver_mx(A, "p@out.example", [ALICE],
                        f"From: p@out.example\r\nTo: {ALICE}\r\nSubject: sse-probe\r\n\r\nx\r\n")).start()
        got, deadline = False, time.time() + 15
        while time.time() < deadline:
            line = resp.readline()
            if b"data: change" in line:
                got = True
                break
            if not line:
                break
        resp.close()
        return got
    run("SSE change pushed on delivery", sse_push)

    phase("cross-server: SMTP submission a.test -> b.test (real auth)")
    submit(A, ALICE, PW, ALICE, [BOB], f"From: {ALICE}\r\nTo: {BOB}\r\nSubject: cross-smtp\r\n\r\nover the wire\r\n")
    raw = imap_find_raw(B, BOB, "cross-smtp")
    run("bob received cross-server message", lambda: bool(raw))
    ar = next((l.lower() for l in raw.splitlines() if l.lower().startswith("authentication-results")), "")
    run("SPF=pass", lambda: ("spf=pass" in ar, ar[:110]))
    run("DKIM=pass", lambda: ("dkim=pass" in ar, ar[:110]))
    run("DMARC=pass", lambda: ("dmarc=pass" in ar, ar[:110]))

    phase("cross-server: JMAP EmailSubmission a.test -> b.test")
    def jmap_submit():
        calls = [
            ["Email/set", {"accountId": ALICE, "create": {"d1": {
                "mailboxIds": {"drafts": True}, "keywords": {"$draft": True},
                "from": [{"email": ALICE}], "to": [{"email": BOB}], "subject": "jmap-submit",
                "textBody": [{"partId": "t", "type": "text/plain"}], "bodyValues": {"t": {"value": "via jmap"}},
            }}}, "0"],
            ["EmailSubmission/set", {"accountId": ALICE, "create": {"s1": {"emailId": "#d1", "identityId": "i0"}},
                                     "onSuccessUpdateEmail": {"#s1": {"mailboxIds/drafts": None, "mailboxIds/sent": True, "keywords/$draft": None}}}, "1"],
        ]
        resp = jmap(A, ALICE, PW, calls, using=("core", "mail", "submission"))
        sub = resp[1][1]
        created = "s1" in (sub.get("created") or {})
        got = find_by_subject(B, BOB, "jmap-submit") is not None
        return created and got, f"created={created}, delivered={got}"
    run("JMAP submission delivered cross-server", jmap_submit)

    phase("attachments end-to-end (webmail send -> cross-server -> download)")
    def attach_e2e():
        data = base64.b64encode(b"PAYLOAD-1234").decode()
        st, _ = http(A, "/api/webmail/send", ALICE, PW, "POST", json.dumps(
            {"to": [BOB], "subject": "attach-e2e", "text": "see attached",
             "attachments": [{"name": "note.txt", "type": "text/plain", "data": data}]}))
        if st != 200:
            return False, f"send HTTP {st}"
        mid = find_by_subject(B, BOB, "attach-e2e")
        if not mid:
            return False, "not delivered"
        g = jcall(B, BOB, PW, "Email/get", {"accountId": BOB, "ids": [mid], "properties": ["attachments"]})
        atts = (g["list"][0].get("attachments") or []) if g["list"] else []
        if not atts:
            return False, "no attachment metadata"
        _, body = http(B, f"/api/webmail/attachment?id={mid}&n=0", BOB, PW)
        return atts[0].get("name") == "note.txt" and body == "PAYLOAD-1234", f"name={atts[0].get('name')}, bytes_ok={body=='PAYLOAD-1234'}"
    run("attachment sent, received, downloaded byte-exact", attach_e2e)

    phase("threading (conversation grouping)")
    def threading_test():
        submit(A, ALICE, PW, ALICE, [BOB], f"From: {ALICE}\r\nTo: {BOB}\r\nSubject: thread-root\r\nMessage-ID: <tr1@a.test>\r\n\r\nroot\r\n")
        submit(A, ALICE, PW, ALICE, [BOB], f"From: {ALICE}\r\nTo: {BOB}\r\nSubject: thread-reply\r\nMessage-ID: <tr2@a.test>\r\nIn-Reply-To: <tr1@a.test>\r\nReferences: <tr1@a.test>\r\n\r\nreply\r\n")
        r1 = find_by_subject(B, BOB, "thread-root")
        r2 = find_by_subject(B, BOB, "thread-reply")
        if not (r1 and r2):
            return False, "both not delivered"
        g = jcall(B, BOB, PW, "Email/get", {"accountId": BOB, "ids": [r1, r2], "properties": ["threadId", "subject"]})
        tids = {m["threadId"] for m in g["list"]}
        return len(tids) == 1, f"threadIds={tids}"
    run("reply shares a thread", threading_test)

    phase("bounce / DSN loop")
    def bounce_test():
        submit(A, ALICE, PW, ALICE, ["nobody@b.test"], f"From: {ALICE}\r\nTo: nobody@b.test\r\nSubject: will-bounce\r\n\r\nx\r\n")
        # mta-b rejects RCPT (550) -> mta-a scheduler bounces -> DSN to alice (local).
        deadline = time.time() + 70
        while time.time() < deadline:
            ids = query_ids(A, ALICE)
            if ids:
                g = jcall(A, ALICE, PW, "Email/get", {"accountId": ALICE, "ids": ids, "properties": ["from", "subject"]})
                for m in g["list"]:
                    frm = (m.get("from") or [{}])[0].get("email", "").lower()
                    if "mailer-daemon" in frm or "undeliver" in (m.get("subject") or "").lower():
                        return True, f"bounce from {frm}"
            time.sleep(3)
        return False, "no DSN received"
    run("undeliverable message bounces back (DSN)", bounce_test)

    phase("vacation auto-responder (cross-server)")
    def vacation_test():
        http(B, "/api/webmail/settings", BOB, PW, "POST", json.dumps({"signature": "", "vacation": {"enabled": True, "subject": "Out of office", "body": "away"}}))
        time.sleep(1)
        submit(A, ALICE, PW, ALICE, [BOB], f"From: {ALICE}\r\nTo: {BOB}\r\nSubject: ping-vacation\r\n\r\nyou around?\r\n")
        got = find_by_subject(A, ALICE, "Out of office", timeout=70) is not None
        # cleanup so later deliveries don't auto-reply
        http(B, "/api/webmail/settings", BOB, PW, "POST", json.dumps({"signature": "", "vacation": {"enabled": False, "subject": "", "body": ""}}))
        return got
    run("vacation auto-reply delivered to sender", vacation_test)

    phase("security boundaries")
    def open_relay():
        try:
            deliver_mx(A, "spammer@evil.example", ["victim@elsewhere.example"], "Subject: relay\r\n\r\nx\r\n")
            return False, "relayed!"
        except smtplib.SMTPException:
            return True
    run("open-relay rejected", open_relay)

    def unknown_rcpt():
        s = smtplib.SMTP(A, MX, timeout=20)
        s.ehlo()
        s.mail("ext@out.example")
        code, _ = s.rcpt("nobody@a.test")
        s.quit()
        return code >= 500, f"code {code}"
    run("unknown recipient rejected at RCPT", unknown_rcpt)

    def submit_spoof():
        try:
            submit(A, ALICE, PW, ALICE, [BOB], f"From: ceo@a.test\r\nTo: {BOB}\r\nSubject: spoof\r\n\r\nx\r\n")
            return False, "spoof accepted!"
        except smtplib.SMTPException:
            return True
    run("submission From-spoof rejected", submit_spoof)

    def jmap_spoof():
        calls = [
            ["Email/set", {"accountId": ALICE, "create": {"d9": {
                "mailboxIds": {"drafts": True}, "from": [{"email": "ceo@a.test"}],
                "to": [{"email": BOB}], "subject": "jmap-spoof",
                "textBody": [{"partId": "t", "type": "text/plain"}], "bodyValues": {"t": {"value": "x"}}}}}, "0"],
            ["EmailSubmission/set", {"accountId": ALICE, "create": {"s9": {"emailId": "#d9", "identityId": "i0"}}}, "1"],
        ]
        resp = jmap(A, ALICE, PW, calls, using=("core", "mail", "submission"))
        sub = resp[1][1]
        return "s9" not in (sub.get("created") or {}), "submission refused"
    run("JMAP submission From-spoof rejected", jmap_spoof)

    def dmarc_fail():
        # Send straight from the runner's IP (not in a.test SPF), unsigned, From a.test.
        deliver_mx(B, ALICE, [BOB], f"From: {ALICE}\r\nTo: {BOB}\r\nSubject: dmarc-fail-probe\r\n\r\nforged\r\n")
        raw = imap_find_raw(B, BOB, "dmarc-fail-probe")
        ar = next((l.lower() for l in raw.splitlines() if l.lower().startswith("authentication-results")), "")
        return "dmarc=fail" in ar, ar[:110]
    run("unauthorized sender gets dmarc=fail", dmarc_fail)

    phase("TLS (STARTTLS + HTTPS) on mta-tls")
    def tls_submit():
        submit(T, CAROL, PW, CAROL, [CAROL], f"From: {CAROL}\r\nTo: {CAROL}\r\nSubject: tls-self\r\n\r\nx\r\n", starttls=True)
        return True
    run("submission STARTTLS + AUTH", tls_submit)

    def tls_mx_imap():
        deliver_mx(T, "ext@out.example", [CAROL], f"From: ext@out.example\r\nTo: {CAROL}\r\nSubject: tls-mx\r\n\r\nx\r\n", starttls=True)
        time.sleep(1)
        c = imaplib.IMAP4(T, IMAP)
        c.starttls(ssl_context=NOVERIFY)
        c.login(CAROL, PW)
        typ, _ = c.select("INBOX")
        _, d = c.search(None, "ALL")
        n = len(d[0].split()) if d and d[0] else 0
        c.logout()
        return typ == "OK" and n >= 1, f"{n} in INBOX over IMAP STARTTLS"
    run("MX STARTTLS receive + IMAP STARTTLS read", tls_mx_imap)
    run("HTTPS JMAP session", lambda: http(T, "/jmap/session", CAROL, PW, tls=True)[0] == 200)

    phase("metrics")
    def metrics_check():
        with urllib.request.urlopen(f"http://{A}:{METRICS}/metrics", timeout=10) as r:
            return "vulos_" in r.read().decode()
    run("Prometheus metrics exposed", metrics_check)

    summary()


if __name__ == "__main__":
    main()
