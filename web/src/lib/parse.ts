export type ParsedAccount = { email: string; password: string };

const emailPass = /^([^\s:]+@[^\s:]+)(?::|----)(.+)$/i;

export function parseAccounts(text: string): ParsedAccount[] {
  const seen = new Set<string>();
  const out: ParsedAccount[] = [];
  for (const raw of text.split(/\r?\n/)) {
    const line = raw.trim();
    if (!line || line.startsWith("#")) continue;
    let email = "";
    let password = "";
    if (line.includes("----")) {
      const idx = line.indexOf("----");
      email = line.slice(0, idx).trim();
      password = line.slice(idx + 4).trim();
    } else {
      const match = line.match(emailPass);
      if (!match) continue;
      email = match[1];
      password = match[2].trim();
    }
    if (!looksLikeEmail(email) || !password) continue;
    const key = email.toLowerCase();
    if (seen.has(key)) continue;
    seen.add(key);
    out.push({ email, password });
  }
  return out;
}

function looksLikeEmail(value: string) {
  const at = value.lastIndexOf("@");
  return at > 0 && value.slice(at + 1).includes(".");
}
