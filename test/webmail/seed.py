#!/usr/bin/env python3
"""Seed a few inbox messages (incl. a hostile XSS-payload message) for the webmail
UI test. Usage: seed.py <host> <mx_port> <user>"""
import smtplib
import sys
import time

host, port, user = sys.argv[1], int(sys.argv[2]), sys.argv[3]
XSS = "window.__pwn=1"
mails = [
    ("Dana Okoro <boss@acme.io>", "Q3 deck + budget",
     "Hi,\n\nAttached are the numbers.\n\n> earlier question\n> answered here\n\nDana"),
    ("Carol <carol@x.test>", "lunch friday?", "noon works"),
    ("GitHub <noreply@github.com>", "[vul-os/vulos-mail] CI passed",
     "green build https://github.com/vul-os/vulos-mail"),
    # Hostile message: XSS payloads in the display name, subject, and body.
    (f'"<img src=x onerror={XSS}>" <evil@x.test>',
     f'XSSPROBE <script>{XSS}</script> <img src=x onerror={XSS}>',
     f'<img src=x onerror="{XSS}"> and <script>{XSS}</script> done'),
]

# Tolerate the server still starting up.
deadline = time.time() + 40
while time.time() < deadline:
    try:
        smtplib.SMTP(host, port, timeout=5).quit()
        break
    except Exception:
        time.sleep(1)

for frm, subj, body in mails:
    s = smtplib.SMTP(host, port, timeout=20)
    s.sendmail("x@y.io", [user],
               f"From: {frm}\r\nTo: {user}\r\nSubject: {subj}\r\nDate: Sun, 21 Jun 2026 10:00:00 +0000\r\n\r\n{body}\r\n")
    s.quit()
print("seeded", len(mails))
