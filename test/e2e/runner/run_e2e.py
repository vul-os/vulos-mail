#!/usr/bin/env python3
"""End-to-end driver for the Dockerized vulos-mail ecosystem.

Exercises every protocol surface on a single instance (mta-a) and verifies a real
cross-server send -> receive between mta-a (a.test) and mta-b (b.test) with
DNS-based SPF/DKIM/DMARC verification over the wire. Exits non-zero if any check
fails.
"""
import base64
import imaplib
import json
import smtplib
import sys
import time
import urllib.request

A, B = "mta-a", "mta-b"
SUBMIT, IMAP, JMAP, MX, METRICS = 587, 143, 8080, 25, 9090
ALICE, BOB, PW = "alice@a.test", "bob@b.test", "pw"

results = []


def check(name, ok, detail=""):
    results.append((name, bool(ok)))
    print(f"  [{'PASS' if ok else 'FAIL'}] {name}" + (f" — {detail}" if detail else ""), flush=True)


def basic(user, pw):
    return "Basic " + base64.b64encode(f"{user}:{pw}".encode()).decode()


def http(host, path, user=None, pw=None, method="GET", body=None, ctype="application/json"):
    url = f"http://{host}:{JMAP}{path}" if not path.startswith("http") else path
    req = urllib.request.Request(url, data=body.encode() if body else None, method=method)
    if user:
        req.add_header("Authorization", basic(user, pw))
    if body:
        req.add_header("Content-Type", ctype)
    try:
        with urllib.request.urlopen(req, timeout=15) as r:
            return r.status, r.read().decode()
    except urllib.error.HTTPError as e:
        return e.code, e.read().decode()


def jmap(host, user, pw, method, args, using=("core", "mail")):
    urns = [f"urn:ietf:params:jmap:{u}" for u in using]
    payload = json.dumps({"using": urns, "methodCalls": [[method, args, "0"]]})
    st, txt = http(host, "/jmap/api", user, pw, "POST", payload)
    if st != 200:
        raise RuntimeError(f"{method} -> HTTP {st}: {txt[:200]}")
    return json.loads(txt)["methodResponses"][0][1]


def metrics_url(host):
    return f"http://{host}:{METRICS}/metrics"


def wait_ready(host, timeout=90):
    deadline = time.time() + timeout
    while time.time() < deadline:
        try:
            st, _ = http(host, "/jmap/session", ALICE if host == A else BOB, PW)
            if st == 200:
                return True
        except Exception:
            pass
        time.sleep(1)
    return False


def deliver_mx(host, mail_from, rcpts, raw):
    s = smtplib.SMTP(host, MX, timeout=20)
    try:
        s.sendmail(mail_from, rcpts, raw)
    finally:
        s.quit()


def main():
    print("== waiting for both MTAs ==", flush=True)
    check("mta-a ready", wait_ready(A))
    check("mta-b ready", wait_ready(B))

    # ---- 1. Inbound MX delivery (a.test) ----
    print("== MX receive / IMAP / JMAP (mta-a) ==", flush=True)
    deliver_mx(A, "ext@out.example", [ALICE],
               f"From: Outsider <ext@out.example>\r\nTo: {ALICE}\r\nSubject: hello a\r\n\r\nbody one\r\n")
    time.sleep(1)
    q = jmap(A, ALICE, PW, "Email/query", {"accountId": ALICE, "filter": {"inMailbox": "inbox"}})
    check("inbound MX delivered to inbox", len(q.get("ids", [])) >= 1, f"{len(q.get('ids', []))} msgs")

    # ---- 2. IMAP ----
    try:
        c = imaplib.IMAP4(A, IMAP)
        c.login(ALICE, PW)
        typ, _ = c.select("INBOX")
        _, data = c.search(None, "ALL")
        n = len(data[0].split()) if data and data[0] else 0
        c.logout()
        check("IMAP login + SELECT + SEARCH", typ == "OK" and n >= 1, f"{n} in INBOX")
    except Exception as e:
        check("IMAP login + SELECT + SEARCH", False, str(e))

    # ---- 3. JMAP get/set ----
    g = jmap(A, ALICE, PW, "Email/get", {"accountId": ALICE, "ids": q["ids"][:1], "properties": ["subject", "preview"]})
    check("JMAP Email/get", g["list"] and g["list"][0].get("subject") == "hello a")
    s = jmap(A, ALICE, PW, "Email/set", {"accountId": ALICE, "update": {q["ids"][0]: {"keywords/$seen": True}}})
    check("JMAP Email/set ($seen)", q["ids"][0] in (s.get("updated") or {}))
    ident = jmap(A, ALICE, PW, "Identity/get", {"accountId": ALICE}, using=("core", "mail", "submission"))
    check("JMAP Identity/get", ident["list"] and ident["list"][0]["email"] == ALICE)

    # ---- 4. Webmail APIs ----
    print("== webmail APIs (mta-a) ==", flush=True)
    st, _ = http(A, "/api/webmail/contacts", ALICE, PW, "POST", json.dumps({"name": "Bob", "email": BOB}))
    _, ctxt = http(A, "/api/webmail/contacts", ALICE, PW)
    check("contacts add+list", st == 200 and BOB in ctxt)
    st, _ = http(A, "/api/webmail/calendar", ALICE, PW, "POST", json.dumps({"summary": "Standup", "start": "2026-06-22T09:00:00Z"}))
    _, caltxt = http(A, "/api/webmail/calendar", ALICE, PW)
    check("calendar add+list", st == 200 and "Standup" in caltxt)
    st, _ = http(A, "/api/webmail/pushtoken", ALICE, PW)
    check("push token mint", st == 200)

    # ---- 5. CalDAV / CardDAV ----
    st, _ = http(A, f"/dav/calendars/{ALICE}/", ALICE, PW, "PROPFIND")
    check("CalDAV PROPFIND", st in (207, 200), f"HTTP {st}")
    st, _ = http(A, f"/dav/addressbooks/{ALICE}/", ALICE, PW, "PROPFIND")
    check("CardDAV PROPFIND", st in (207, 200), f"HTTP {st}")

    # ---- 6. Security: open relay + unknown rcpt + submission spoof ----
    print("== security boundaries ==", flush=True)
    try:
        deliver_mx(A, "spammer@evil.example", ["victim@elsewhere.example"], "Subject: relay\r\n\r\nx\r\n")
        check("open-relay rejected", False, "relayed!")
    except smtplib.SMTPException:
        check("open-relay rejected", True)
    try:
        s = smtplib.SMTP(A, MX, timeout=20)
        s.mail("ext@out.example")
        code, _ = s.rcpt("nobody@a.test")
        s.quit()
        check("unknown recipient rejected at RCPT", code >= 500, f"code {code}")
    except smtplib.SMTPException as e:
        check("unknown recipient rejected at RCPT", True, str(e))
    try:
        s = smtplib.SMTP(A, SUBMIT, timeout=20)
        s.login(ALICE, PW)
        s.sendmail(ALICE, [BOB], f"From: ceo@a.test\r\nTo: {BOB}\r\nSubject: spoof\r\n\r\nx\r\n")
        s.quit()
        check("submission From-spoof rejected", False, "spoof accepted!")
    except smtplib.SMTPException:
        check("submission From-spoof rejected", True)

    # ---- 7. CROSS-SERVER send -> receive with SPF/DKIM/DMARC over the wire ----
    print("== cross-server delivery a.test -> b.test (real DNS auth) ==", flush=True)
    subj = "cross-server-hello"
    s = smtplib.SMTP(A, SUBMIT, timeout=20)
    s.login(ALICE, PW)
    s.sendmail(ALICE, [BOB], f"From: {ALICE}\r\nTo: {BOB}\r\nSubject: {subj}\r\n\r\nhi bob, over the wire\r\n")
    s.quit()

    raw = ""
    deadline = time.time() + 60
    while time.time() < deadline:
        try:
            c = imaplib.IMAP4(B, IMAP)
            c.login(BOB, PW)
            c.select("INBOX")
            _, data = c.search(None, "ALL")
            ids = data[0].split() if data and data[0] else []
            for i in ids:
                _, fd = c.fetch(i, "(BODY[])")
                body = fd[0][1].decode(errors="replace") if fd and fd[0] else ""
                if subj in body:
                    raw = body
                    break
            c.logout()
        except Exception:
            pass
        if raw:
            break
        time.sleep(2)

    check("cross-server: bob received the message", bool(raw))
    ar = ""
    for line in raw.splitlines():
        if line.lower().startswith("authentication-results"):
            ar = line.lower()
            break
    check("cross-server: SPF pass", "spf=pass" in ar, ar[:120])
    check("cross-server: DKIM pass", "dkim=pass" in ar, ar[:120])
    check("cross-server: DMARC pass", "dmarc=pass" in ar, ar[:120])

    # ---- 8. Metrics ----
    try:
        with urllib.request.urlopen(metrics_url(A), timeout=10) as r:
            mtxt = r.read().decode()
        check("Prometheus metrics exposed", "vulos_" in mtxt)
    except Exception as e:
        check("Prometheus metrics exposed", False, str(e))

    # ---- summary ----
    passed = sum(1 for _, ok in results if ok)
    total = len(results)
    print(f"\n=== {passed}/{total} checks passed ===", flush=True)
    sys.exit(0 if passed == total else 1)


if __name__ == "__main__":
    main()
