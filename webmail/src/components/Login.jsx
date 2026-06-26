import { useEffect, useRef, useState } from "react";

// Solve an Altcha proof-of-work challenge: find n with SHA-256(salt+n)==challenge.
async function solveAltcha(ch) {
  const enc = new TextEncoder();
  for (let n = 0; n <= ch.maxnumber; n++) {
    const buf = await crypto.subtle.digest("SHA-256", enc.encode(ch.salt + n));
    const hex = [...new Uint8Array(buf)].map((b) => b.toString(16).padStart(2, "0")).join("");
    if (hex === ch.challenge) {
      return btoa(JSON.stringify({ algorithm: ch.algorithm, challenge: ch.challenge, number: n, salt: ch.salt, signature: ch.signature }));
    }
  }
  throw new Error("could not solve challenge");
}

export default function Login({ onLogin }) {
  const [mode, setMode] = useState("login"); // "login" | "signup"
  const [user, setUser] = useState("");
  const [pass, setPass] = useState("");
  const [loginErr, setLoginErr] = useState("");
  const [loginBusy, setLoginBusy] = useState(false);

  const [handle, setHandle] = useState("");
  const [spPass, setSpPass] = useState("");
  const [signupErr, setSignupErr] = useState("");
  const [signupBusy, setSignupBusy] = useState(false);

  const userRef = useRef(null);
  const handleRef = useRef(null);

  useEffect(() => {
    if (mode === "login") userRef.current?.focus();
    else handleRef.current?.focus();
  }, [mode]);

  // Perform an actual sign-in with explicit creds (used by both login submit and
  // the post-signup seamless sign-in). Establishes the server-side webmail
  // session (HttpOnly cookie) that the mail-ui's /v1 calls then ride.
  async function doSignIn(u, p) {
    const r = await fetch("/api/webmail/login", {
      method: "POST",
      headers: { "Content-Type": "application/json" },
      credentials: "include",
      body: JSON.stringify({ user: u, password: p }),
    });
    if (!r.ok) {
      const j = await r.json().catch(() => ({}));
      throw new Error(j.error || (r.status === 401 ? "Invalid credentials" : "Sign-in failed (" + r.status + ")"));
    }
    onLogin();
  }

  async function submitLogin(e) {
    e.preventDefault();
    const u = user.trim();
    setLoginBusy(true); setLoginErr("");
    try {
      await doSignIn(u, pass);
    } catch (ex) {
      setLoginErr(ex.message);
      setLoginBusy(false);
    }
  }

  async function submitSignup(e) {
    e.preventDefault();
    const h = handle.trim().toLowerCase();
    setSignupBusy(true); setSignupErr("");
    try {
      const ch = await fetch("/api/signup/challenge").then((r) => { if (!r.ok) throw new Error("signup unavailable"); return r.json(); });
      const solution = await solveAltcha(ch);
      const res = await fetch("/api/signup", { method: "POST", headers: { "Content-Type": "application/json" }, body: JSON.stringify({ handle: h, password: spPass, solution }) });
      if (!res.ok) { const j = await res.json().catch(() => ({})); throw new Error(j.error || "could not create account"); }
      const { address } = await res.json();
      // Seamlessly sign in to the new account.
      await doSignIn(address, spPass);
    } catch (ex) {
      setSignupErr(ex.message);
      setSignupBusy(false);
    }
  }

  return (
    <section id="login" className="login">
      <div className="login-card">
        <div className="brand">
          <span className="logo" aria-hidden="true" />
          <span className="wordmark">Vulos Mail</span>
        </div>
        <p className="login-sub">Sovereign mail. You own it.</p>

        <form id="login-form" autoComplete="on" hidden={mode !== "login"} onSubmit={submitLogin}>
          <label className="field">
            <span>Email</span>
            <input id="login-user" ref={userRef} type="email" inputMode="email" autoComplete="username"
              placeholder="you@vulos.to" required value={user} onChange={(e) => setUser(e.target.value)} />
          </label>
          <label className="field">
            <span>Password</span>
            <input id="login-pass" type="password" autoComplete="current-password" placeholder="••••••••"
              required value={pass} onChange={(e) => setPass(e.target.value)} />
          </label>
          <button className="btn btn-primary btn-block" type="submit" id="login-btn" disabled={loginBusy}>
            {loginBusy ? "Signing in…" : "Sign in"}
          </button>
          {loginErr && <p className="login-err" id="login-err">{loginErr}</p>}
        </form>

        {mode === "login" && (
          <p className="login-alt">New to Vulos Mail? <a href="#" id="show-signup" onClick={(e) => { e.preventDefault(); setMode("signup"); }}>Create a free account</a></p>
        )}

        <form id="signup-form" autoComplete="on" hidden={mode !== "signup"} onSubmit={submitSignup}>
          <label className="field">
            <span>Choose your address</span>
            <input id="signup-handle" ref={handleRef} type="text" autoComplete="username" placeholder="yourname"
              pattern="[a-z0-9][a-z0-9._-]{2,31}" required value={handle} onChange={(e) => setHandle(e.target.value)} />
          </label>
          <label className="field">
            <span>Password</span>
            <input id="signup-pass" type="password" autoComplete="new-password" placeholder="at least 8 characters"
              minLength={8} required value={spPass} onChange={(e) => setSpPass(e.target.value)} />
          </label>
          <button className="btn btn-primary btn-block" type="submit" id="signup-btn" disabled={signupBusy}>
            {signupBusy ? "Creating account…" : "Create free account"}
          </button>
          {signupErr && <p className="login-err" id="signup-err">{signupErr}</p>}
          <p className="login-alt"><a href="#" id="show-login" onClick={(e) => { e.preventDefault(); setMode("login"); setSignupErr(""); }}>Back to sign in</a></p>
        </form>
      </div>
      <div className="login-glow" aria-hidden="true" />
    </section>
  );
}
