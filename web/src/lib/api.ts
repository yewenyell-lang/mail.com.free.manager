import type { AccountRow } from "./db";
import { db } from "./db";
import type { Folder, IncomingData, MailMessage, Session } from "./types";

type Envelope<T> = {
  session?: Session;
  data?: T;
  html?: string;
  user?: unknown;
  error?: string;
  ok?: boolean;
  filename?: string;
  contentType?: string;
  base64data?: string;
};

function sessionBody(account: AccountRow, extra: Record<string, unknown> = {}) {
  return {
    email: account.email,
    password: account.password,
    accessToken: account.accessToken,
    refreshToken: account.refreshToken,
    ...extra,
  };
}

async function persistSession(email: string, session?: Session) {
  if (!session?.accessToken) return;
  await db.accounts.update(email, {
    accessToken: session.accessToken,
    refreshToken: session.refreshToken,
    expiresAt: session.expiresAt,
    status: "ok",
    lastError: "",
    lastLoginAt: Date.now(),
  });
}

async function request<T>(path: string, account: AccountRow, extra: Record<string, unknown> = {}): Promise<Envelope<T>> {
  const res = await fetch(path, {
    method: "POST",
    headers: { "Content-Type": "application/json" },
    body: JSON.stringify(sessionBody(account, extra)),
  });
  const json = (await res.json()) as Envelope<T>;
  if (!res.ok) {
    throw new Error(json.error || `${res.status}`);
  }
  await persistSession(account.email, json.session);
  return json;
}

const loginLocks = new Map<string, Promise<unknown>>();

export function hasUsableSession(account: AccountRow) {
  return Boolean(account.accessToken);
}

export async function loginAccount(account: AccountRow, opts?: { force?: boolean }) {
  const fresh = (await db.accounts.get(account.email)) || account;
  if (!opts?.force && fresh.accessToken) return;
  const running = loginLocks.get(fresh.email);
  if (running) return running;
  const job = (async () => {
    const current = (await db.accounts.get(fresh.email)) || fresh;
    if (!opts?.force && current.accessToken) return;
    await db.accounts.update(current.email, { status: "busy", lastError: "" });
    try {
      const json = await request("/api/auth/login", current);
      await persistSession(current.email, json.session);
      return json;
    } catch (error) {
      const message = error instanceof Error ? error.message : String(error);
      await db.accounts.update(current.email, { status: "fail", lastError: message });
      throw error;
    }
  })();
  loginLocks.set(fresh.email, job);
  try {
    return await job;
  } finally {
    loginLocks.delete(fresh.email);
  }
}

export async function listMail(account: AccountRow, extra: Record<string, unknown> = {}) {
  const json = await request<IncomingData>("/api/mail/list", account, extra);
  if (json.data) {
    await db.mailLists.put({
      email: account.email,
      folder: String(extra.folderId || "incoming"),
      mails: "mail" in json.data ? json.data.mail || [] : [],
      updatedAt: Date.now(),
    });
  }
  return json.data;
}

export async function searchMail(account: AccountRow, query: string) {
  const json = await request<{ mail: MailMessage[] }>("/api/mail/search", account, { query, amount: 50 });
  return json.data;
}

export async function getBody(account: AccountRow, mailId: string, markRead = false) {
  const json = await request<never>("/api/mail/body", account, { mailId, markRead });
  return json.html || "";
}

export async function downloadAttachment(account: AccountRow, mailId: string, attachmentId: string) {
  const json = await request<never>("/api/mail/attachment", account, { mailId, attachmentId });
  return {
    filename: json.filename || "attachment",
    contentType: json.contentType || "application/octet-stream",
    base64data: json.base64data || "",
  };
}

export async function getPreview(account: AccountRow, mailIds: string[]) {
  const json = await request<Array<{ mailIdentifier: string; preview: string }>>("/api/mail/preview", account, { mailIds });
  return json.data || [];
}

export async function sendMail(account: AccountRow, payload: Record<string, unknown>) {
  return request("/api/mail/send", account, payload);
}

export async function replyMail(account: AccountRow, payload: Record<string, unknown>) {
  return request("/api/mail/reply", account, payload);
}

export async function forwardMail(account: AccountRow, payload: Record<string, unknown>) {
  return request("/api/mail/forward", account, payload);
}

export async function runAction(
  kind: "read" | "unread" | "star" | "unstar" | "spam" | "trash" | "delete" | "inbox",
  account: AccountRow,
  mailIds: string[],
) {
  return request(`/api/actions/${kind}`, account, { mailIds });
}

export async function getFolders(account: AccountRow) {
  const json = await request<{ folders?: Folder[] }>("/api/mail/folders", account);
  return json.data?.folders || [];
}

export async function getAliases(account: AccountRow) {
  const json = await request<{ mailaddresslist?: Array<{ address: string; displayName?: string; defaultSenderAddress?: boolean }> }>(
    "/api/account/aliases",
    account,
  );
  return json.data?.mailaddresslist || [];
}

export function randomMailPassword(oldPassword = "") {
  const upper = "ABCDEFGHJKLMNPQRSTUVWXYZ";
  const lower = "abcdefghijkmnopqrstuvwxyz";
  const digits = "23456789";
  const pool = lower + digits;
  for (let attempt = 0; attempt < 8; attempt++) {
    const len = 12 + Math.floor(Math.random() * 5);
    const rest = [lower[Math.floor(Math.random() * lower.length)], digits[Math.floor(Math.random() * digits.length)]];
    while (rest.length < len - 1) rest.push(pool[Math.floor(Math.random() * pool.length)]);
    for (let i = rest.length - 1; i > 0; i--) {
      const j = Math.floor(Math.random() * (i + 1));
      [rest[i], rest[j]] = [rest[j], rest[i]];
    }
    const password = upper[Math.floor(Math.random() * upper.length)] + rest.join("");
    if (password !== oldPassword) return password;
  }
  return `A${Date.now().toString(36).slice(-9)}`;
}

export async function changePassword(account: AccountRow, newPassword: string) {
  const ctrl = new AbortController();
  const timer = window.setTimeout(() => ctrl.abort(), 120000);
  try {
    const res = await fetch("/api/account/password", {
      method: "POST",
      headers: { "Content-Type": "application/json" },
      body: JSON.stringify(sessionBody(account, { newPassword })),
      signal: ctrl.signal,
    });
    const json = (await res.json()) as Envelope<never>;
    if (!res.ok) {
      throw new Error(json.error || `${res.status}`);
    }
    await db.accounts.update(account.email, { password: newPassword, lastError: "" });
  } catch (error) {
    const message =
      error instanceof DOMException && error.name === "AbortError"
        ? "改密超时"
        : error instanceof Error
          ? error.message
          : String(error);
    await db.accounts.update(account.email, { lastError: message });
    throw new Error(message);
  } finally {
    window.clearTimeout(timer);
  }
}

export async function mapPool<T>(items: T[], limit: number, worker: (item: T, index: number) => Promise<void>) {
  let next = 0;
  const runners = Array.from({ length: Math.min(limit, items.length) }, async () => {
    while (next < items.length) {
      const current = next;
      next += 1;
      await worker(items[current], current);
    }
  });
  await Promise.all(runners);
}
