import { describe, it, expect } from "vitest";
import { sanitizeHTML, fmtBody, renderEmailHTML, esc, initials, fmtBytes } from "./util.js";

describe("sanitizeHTML — XSS inertness", () => {
  it("drops <script> entirely", () => {
    const out = sanitizeHTML('hi <script>window.__pwn=1</script> bye');
    expect(out).not.toContain("<script");
    expect(out).not.toContain("__pwn");
  });
  it("drops <img onerror>", () => {
    const out = sanitizeHTML('<img src=x onerror="window.__pwn=1">');
    expect(out).not.toContain("<img");
    expect(out).not.toContain("onerror");
  });
  it("keeps safe formatting + http links, scrubs js: hrefs", () => {
    const out = sanitizeHTML('<b>bold</b> <a href="javascript:alert(1)">x</a> <a href="https://ok">y</a>');
    expect(out).toContain("<b>bold</b>");
    expect(out).not.toContain("javascript:");
    expect(out).toContain('href="https://ok"');
    expect(out).toContain('rel="noopener noreferrer"');
  });
});

describe("renderEmailHTML / fmtBody", () => {
  it("escapes hostile plain text", () => {
    const out = renderEmailHTML({ bodyValues: { 1: { value: '<img src=x onerror=window.__pwn=1>', isHTML: false } } });
    expect(out).not.toContain("<img");
    expect(out).toContain("&lt;img");
  });
  it("sanitizes hostile HTML bodies", () => {
    const out = renderEmailHTML({ bodyValues: { 1: { value: '<p>ok</p><script>window.__pwn=1</script>', isHTML: true } } });
    expect(out).toContain("<p>ok</p>");
    expect(out).not.toContain("<script");
  });
  it("wraps quoted lines in blockquote", () => {
    expect(fmtBody("> quoted\nplain")).toContain("<blockquote>");
  });
});

describe("misc helpers", () => {
  it("esc escapes the dangerous set", () => {
    expect(esc('<a&"')).toBe("&lt;a&amp;&quot;");
  });
  it("initials", () => {
    expect(initials("Dana Okoro")).toBe("DO");
    expect(initials("")).toBe("?");
  });
  it("fmtBytes", () => {
    expect(fmtBytes(512)).toBe("512 B");
    expect(fmtBytes(2048)).toBe("2 KB");
  });
});
