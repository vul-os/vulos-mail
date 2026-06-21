#!/usr/bin/env python3
"""Extended-matrix driver: alternate backends (SQLite, S3), rspamd spam scanning,
and a concurrency/load burst. CRASH_CHECK=1 runs only the post-crash probe.
"""
import base64
import concurrent.futures
import json
import os
import smtplib
import sys
import time
import urllib.request

SQLITE, S3, RSPAMD = "mta-sqlite", "mta-s3", "mta-rspamd"
MX, JMAP = 2525, 2080
USER = {SQLITE: "u@sqlite.test", S3: "u@s3.test", RSPAMD: "u@rspamd.test"}
PW = "pw"
GTUBE = "XJS*C4JDBQADN1.NSBN3*2IDNEN*GTUBE-STANDARD-ANTI-UBE-TEST-EMAIL*C.34X"

results = []


def check(name, ok, detail=""):
    results.append((name, bool(ok)))
    print(f"  [{'PASS' if ok else 'FAIL'}] {name}" + (f" — {detail}" if detail else ""), flush=True)


def basic(u, p):
    return "Basic " + base64.b64encode(f"{u}:{p}".encode()).decode()


def jmap(host, user, method, args):
    payload = json.dumps({"using": ["urn:ietf:params:jmap:core", "urn:ietf:params:jmap:mail"],
                          "methodCalls": [[method, args, "0"]]})
    req = urllib.request.Request(f"http://{host}:{JMAP}/jmap/api", data=payload.encode(), method="POST")
    req.add_header("Authorization", basic(user, PW))
    req.add_header("Content-Type", "application/json")
    with urllib.request.urlopen(req, timeout=20) as r:
        return json.loads(r.read())["methodResponses"][0][1]


def inbox_ids(host, user):
    return jmap(host, user, "Email/query", {"accountId": user, "filter": {"inMailbox": "inbox"}}).get("ids", [])


def wait_ready(host, user, timeout=90):
    deadline = time.time() + timeout
    while time.time() < deadline:
        try:
            req = urllib.request.Request(f"http://{host}:{JMAP}/jmap/session")
            req.add_header("Authorization", basic(user, PW))
            with urllib.request.urlopen(req, timeout=10) as r:
                if r.status == 200:
                    return True
        except Exception:
            pass
        time.sleep(1)
    return False


def deliver(host, mail_from, rcpt, raw):
    s = smtplib.SMTP(host, MX, timeout=20)
    try:
        s.sendmail(mail_from, [rcpt], raw)
    finally:
        try:
            s.quit()
        except Exception:
            pass


def msg(frm, to, subj, body):
    return f"From: {frm}\r\nTo: {to}\r\nSubject: {subj}\r\n\r\n{body}\r\n"


def realmsg(frm, to, subj, body):
    # A well-formed message (Date + Message-ID + Content-Type) so a benign mail
    # scores low in rspamd instead of being flagged for missing headers.
    return (f"From: {frm}\r\nTo: {to}\r\nSubject: {subj}\r\n"
            "Date: Sun, 21 Jun 2026 10:00:00 +0000\r\n"
            f"Message-ID: <clean-{subj}@out.example>\r\n"
            "MIME-Version: 1.0\r\nContent-Type: text/plain; charset=utf-8\r\n"
            f"\r\n{body}\r\n")


def crash_probe():
    print("== post-crash persistence (mta-sqlite) ==", flush=True)
    ok = wait_ready(SQLITE, USER[SQLITE])
    check("mta-sqlite ready after SIGKILL", ok)
    if ok:
        n = len(inbox_ids(SQLITE, USER[SQLITE]))
        check("inbox survived hard crash (SQLite durability)", n >= 1, f"{n} msgs")
    summary()


def summary():
    p = sum(1 for _, ok in results if ok)
    print(f"\n=== {p}/{len(results)} checks passed ===", flush=True)
    sys.exit(0 if p == len(results) else 1)


def main():
    if os.environ.get("CRASH_CHECK"):
        crash_probe()
        return

    print("== readiness ==", flush=True)
    for h in (SQLITE, S3, RSPAMD):
        check(f"{h} ready", wait_ready(h, USER[h]))

    print("== SQLite event-log backend ==", flush=True)
    deliver(SQLITE, "ext@out.example", USER[SQLITE], msg("ext@out.example", USER[SQLITE], "sqlite-msg", "stored in sqlite"))
    time.sleep(1)
    check("SQLite: deliver + read back", len(inbox_ids(SQLITE, USER[SQLITE])) >= 1)

    print("== S3 (minio) blob backend ==", flush=True)
    deliver(S3, "ext@out.example", USER[S3], msg("ext@out.example", USER[S3], "s3-msg", "BODY-IN-S3-42"))
    time.sleep(1)

    def s3_body():
        ids = inbox_ids(S3, USER[S3])
        if not ids:
            return False, "not delivered"
        g = jmap(S3, USER[S3], "Email/get", {"accountId": USER[S3], "ids": ids[:1], "properties": ["bodyValues", "preview"]})
        m = g["list"][0]
        body = (m.get("preview") or "") + json.dumps(m.get("bodyValues") or {})
        return "BODY-IN-S3-42" in body, "body fetched from S3"
    check("S3: body stored in minio + read back", *s3_body())

    print("== rspamd spam scanning ==", flush=True)
    deliver(RSPAMD, "friend@out.example", USER[RSPAMD], realmsg("friend@out.example", USER[RSPAMD], "clean", "hello, normal mail"))
    time.sleep(2)
    check("rspamd: clean mail accepted -> inbox", len(inbox_ids(RSPAMD, USER[RSPAMD])) >= 1)

    def gtube_blocked():
        before = len(inbox_ids(RSPAMD, USER[RSPAMD]))
        try:
            deliver(RSPAMD, "spammer@out.example", USER[RSPAMD], msg("spammer@out.example", USER[RSPAMD], "spam", GTUBE))
            # accepted: must NOT be in inbox (routed to spam) — check inbox unchanged
            time.sleep(1)
            after = len(inbox_ids(RSPAMD, USER[RSPAMD]))
            return after == before, "GTUBE not in inbox"
        except smtplib.SMTPException:
            return True, "GTUBE rejected at SMTP"
    check("rspamd: GTUBE spam blocked from inbox", *gtube_blocked())

    print("== concurrency / load (200 deliveries -> mta-sqlite) ==", flush=True)
    base = len(inbox_ids(SQLITE, USER[SQLITE]))
    N = 200

    def one(i):
        deliver(SQLITE, f"s{i}@out.example", USER[SQLITE], msg(f"s{i}@out.example", USER[SQLITE], f"load-{i}", f"body {i}"))
    with concurrent.futures.ThreadPoolExecutor(max_workers=32) as ex:
        list(ex.map(one, range(N)))
    # allow async folding to settle
    deadline, got = time.time() + 30, 0
    while time.time() < deadline:
        got = len(inbox_ids(SQLITE, USER[SQLITE]))
        if got >= base + N:
            break
        time.sleep(1)
    check("load: all 200 concurrent deliveries landed", got >= base + N, f"{got - base}/{N}")

    summary()


if __name__ == "__main__":
    main()
