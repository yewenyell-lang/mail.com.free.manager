import { useLiveQuery } from "dexie-react-hooks";
import DOMPurify from "dompurify";
import { Copy, Github, Inbox, LogIn, LoaderCircle, MailPlus, Monitor, Moon, RefreshCw, Search, Sun, Upload } from "lucide-react";
import BrandMark from "./BrandMark";
import { lazy, Suspense, useEffect, useMemo, useRef, useState, type ReactNode } from "react";

const Compose = lazy(() => import("./Compose"));
import {
  downloadAttachment,
  getBody,
  getFolders,
  getPreview,
  listMail,
  hasUsableSession,
  loginAccount,
  mapPool,
  runAction,
  searchMail,
} from "./lib/api";
import { extractCode } from "./lib/code";
import { cacheFolderOf, db, recoverAccountsIfEmpty, type AccountRow } from "./lib/db";
import { parseAccounts } from "./lib/parse";
import { applyThemePref, cycleTheme, readThemePref, themeLabel, type ThemePref } from "./lib/theme";
import type { Folder, MailAttachment, MailBox, MailMessage } from "./lib/types";

function mailId(mail: MailMessage) {
  return mail.attribute?.mailIdentifier || mail.mailURI || "";
}

function mailTime(mail: MailMessage) {
  return mail.mailHeader?.date || 0;
}

function sortMails(list: MailMessage[]) {
  return [...list].sort((a, b) => mailTime(b) - mailTime(a));
}

function fmtTime(ms?: number) {
  if (!ms) return "";
  const d = new Date(ms);
  return `${d.getMonth() + 1}/${d.getDate()} ${d.getHours().toString().padStart(2, "0")}:${d.getMinutes().toString().padStart(2, "0")}`;
}

function flattenFolders(folders: Folder[]): Folder[] {
  const out: Folder[] = [];
  const walk = (items: Folder[]) => {
    for (const item of items) {
      out.push(item);
      if (item.folders?.length) walk(item.folders);
    }
  };
  walk(folders);
  return out;
}

function folderTypeOf(mail: MailMessage) {
  return (mail.sourceFolder?.folderType || mail.attribute?.folderType || "").toUpperCase();
}

function isSentMail(mail: MailMessage) {
  const type = folderTypeOf(mail);
  const name = (mail.sourceFolder?.folderName || "").toLowerCase();
  return type === "SENT" || name === "sent" || name.includes("已发送");
}

function isTrashMail(mail: MailMessage) {
  return folderTypeOf(mail) === "TRASH";
}

function isSpamMail(mail: MailMessage) {
  return folderTypeOf(mail) === "SPAM";
}

function findFolderByType(folders: Folder[], type: string) {
  const want = type.toUpperCase();
  return flattenFolders(folders).find((folder) => (folder.attribute?.folderType || "").toUpperCase() === want);
}

function attachmentId(item: MailAttachment) {
  return item.attachmentURI || "";
}

function cidKey(value?: string) {
  return (value || "").replaceAll("<", "").replaceAll(">", "").trim();
}

function rewriteCid(html: string, contentId: string, dataUrl: string) {
  const id = cidKey(contentId);
  if (!id) return html;
  return html.replaceAll(`cid:${id}`, dataUrl).replaceAll(`CID:${id}`, dataUrl);
}

function sanitizeBody(raw: string) {
  return DOMPurify.sanitize(raw || "<p>无正文</p>", { ADD_DATA_URI_TAGS: ["img"] });
}

function counterpart(mail: MailMessage) {
  if (isSentMail(mail)) {
    const to = mail.mailHeader?.to?.filter(Boolean) || [];
    return to.length ? `发给 ${to.join(", ")}` : "已发送";
  }
  return mail.mailHeader?.from || "";
}

export default function App() {
  const accounts = useLiveQuery(() => db.accounts.toArray()) || [];
  const [selectedEmail, setSelectedEmail] = useState<string>("");
  const [mails, setMails] = useState<MailMessage[]>([]);
  const [selectedMail, setSelectedMail] = useState<MailMessage | null>(null);
  const [html, setHtml] = useState("");
  const [preview, setPreview] = useState("");
  const [query, setQuery] = useState("");
  const [busy, setBusy] = useState<{ text: string; pane?: "mail" | "detail" | "global"; current?: number; total?: number } | null>(null);
  const progressRef = useRef(0);
  const [toast, setToast] = useState<{ text: string; kind: "ok" | "error" } | null>(null);
  const [importOpen, setImportOpen] = useState(false);
  const [composeOpen, setComposeOpen] = useState<"send" | "reply" | "forward" | null>(null);
  const [importText, setImportText] = useState("");
  const [accountFilter, setAccountFilter] = useState("");
  const [selectedIds, setSelectedIds] = useState<Set<string>>(new Set());
  const [pickedEmails, setPickedEmails] = useState<Set<string>>(new Set());
  const [box, setBox] = useState<MailBox>("inbox");
  const [accountMenu, setAccountMenu] = useState<{ email: string; x: number; y: number } | null>(null);
  const [themePref, setThemePref] = useState<ThemePref>(() => readThemePref());

  const selected = accounts.find((a) => a.email === selectedEmail);
  const composeAccount = selected || accounts[0];
  const code = extractCode(selectedMail?.mailHeader?.subject, preview, html.replace(/<[^>]+>/g, " "));
  const filteredAccounts = accounts.filter((a) => a.email.toLowerCase().includes(accountFilter.toLowerCase()));
  const pickedAccounts = accounts.filter((a) => pickedEmails.has(a.email));

  const visibleMails = useMemo(() => {
    if (box === "sent") return mails.filter(isSentMail);
    if (box === "trash") return mails.filter(isTrashMail);
    if (box === "spam") return mails.filter(isSpamMail);
    if (box === "inbox") return mails.filter((mail) => !isSentMail(mail) && !isTrashMail(mail) && !isSpamMail(mail));
    return mails;
  }, [box, mails]);

  const unread = useMemo(
    () => visibleMails.filter((m) => !isSentMail(m) && m.attribute?.read === false).length,
    [visibleMails],
  );

  function patchRead(id: string) {
    const mark = (item: MailMessage) =>
      mailId(item) === id ? { ...item, attribute: { ...item.attribute, read: true } } : item;
    setMails((list) => list.map(mark));
    setSelectedMail((item) => (item ? mark(item) : item));
    if (selectedEmail) {
      const folderKey = cacheFolderOf(box);
      void db.mailLists.get([selectedEmail, folderKey]).then((cached) => {
        if (!cached) return;
        void db.mailLists.put({
          ...cached,
          mails: ((cached.mails as MailMessage[]) || []).map(mark),
        });
      });
    }
  }

  useEffect(() => {
    void recoverAccountsIfEmpty();
  }, []);

  useEffect(() => {
    applyThemePref(themePref);
  }, [themePref]);

  function ping(message: string) {
    const errorWords = ["失败", "错误", "超过", "不能", "无法", "没有", "先", "不可"];
    const kind = errorWords.some((word) => message.includes(word)) ? "error" : "ok";
    setToast({ text: message, kind });
    window.setTimeout(() => setToast(null), kind === "error" ? 3500 : 2400);
  }

  async function importNow() {
    const parsed = parseAccounts(importText);
    if (!parsed.length) {
      ping("没有解析到账号");
      return;
    }
    const emails = parsed.map((item) => item.email);
    const existing = await db.accounts.bulkGet(emails);
    const prev = new Map(existing.filter(Boolean).map((row) => [row!.email, row!]));
    await db.accounts.bulkPut(
      parsed.map((item) => {
        const old = prev.get(item.email);
        return {
          ...old,
          email: item.email,
          password: item.password,
          status: old?.accessToken ? old.status : "unknown",
          tags: old?.tags || [],
        };
      }),
    );
    setImportOpen(false);
    setImportText("");
    ping(`已导入 ${parsed.length} 个账号`);
  }

  async function reloginOne(account: AccountRow) {
    setAccountMenu(null);
    setBusy({ text: `正在重新登录 ${account.email}`, pane: "global" });
    try {
      await loginAccount(account, { force: true });
      ping(`${account.email} 已重新登录`);
    } catch (error) {
      ping(error instanceof Error ? error.message : "重新登录失败");
    } finally {
      setBusy(null);
    }
  }

  async function copyAccount(account: AccountRow, withPassword = false) {
    setAccountMenu(null);
    await navigator.clipboard.writeText(withPassword ? `${account.email}----${account.password}` : account.email);
    ping(withPassword ? "已复制邮箱和密码" : "已复制邮箱");
  }

  async function loginOne(account: AccountRow) {
    const current = (await db.accounts.get(account.email)) || account;
    if (hasUsableSession(current)) {
      await fetchMail(current, "", { force: true });
      return;
    }
    setBusy({ text: `正在登录 ${current.email}`, pane: "global" });
    try {
      await loginAccount(current);
      ping(`${current.email} 登录成功`);
    } catch (error) {
      ping(error instanceof Error ? error.message : "登录失败");
    } finally {
      setBusy(null);
    }
  }

  async function loginAll() {
    const chosen = pickedAccounts.length ? pickedAccounts : [];
    if (!chosen.length) {
      ping("先勾选要登录的账号");
      return;
    }
    const targets = chosen.filter((account) => !account.accessToken);
    if (!targets.length) {
      ping("选中账号均已登录");
      return;
    }
    const total = targets.length;
    progressRef.current = 0;
    setBusy({ text: `批量登录 0/${total}`, pane: "global", current: 0, total });
    await mapPool(targets, 3, async (account) => {
      try {
        await loginAccount(account);
      } catch {
        /* status already stored */
      } finally {
        progressRef.current += 1;
        const current = progressRef.current;
        setBusy({ text: `批量登录 ${current}/${total}`, pane: "global", current, total });
      }
    });
    setBusy(null);
    ping("批量登录结束");
  }

  async function fetchMail(
    account: AccountRow,
    q = "",
    opts?: { keepSelection?: boolean; box?: MailBox; quiet?: boolean; force?: boolean },
  ) {
    const view = opts?.box ?? box;
    const keepId = opts?.keepSelection && selectedMail ? mailId(selectedMail) : "";
    const keepHtml = keepId ? html : "";
    const keepPreview = keepId ? preview : "";
    const folderKey = cacheFolderOf(view);
    setSelectedEmail(account.email);

    if (!opts?.force && !q.trim()) {
      const cached = await db.mailLists.get([account.email, folderKey]);
      if (cached) {
        const list = sortMails((cached.mails as MailMessage[]) || []);
        setMails(list);
        if (!opts?.keepSelection) {
          setSelectedMail(null);
          setHtml("");
          setPreview("");
        } else if (keepId) {
          const found = list.find((mail) => mailId(mail) === keepId) || null;
          setSelectedMail(found);
          if (found) {
            setHtml(keepHtml);
            setPreview(keepPreview);
          } else {
            setHtml("");
            setPreview("");
          }
        }
        return;
      }
    }

    setBusy({ text: `正在收信 ${account.email}`, pane: "mail" });
    if (!opts?.keepSelection) {
      setMails([]);
      setSelectedMail(null);
      setHtml("");
      setPreview("");
    }
    try {
      let current = (await db.accounts.get(account.email)) || account;
      if (!current.accessToken) {
        await loginAccount(current);
        current = (await db.accounts.get(account.email)) || current;
      }
      let list: MailMessage[] = [];
      if (q.trim() && view !== "trash" && view !== "spam") {
        const data = await searchMail(current, q.trim());
        list = sortMails(data?.mail || []);
      } else if (view === "trash" || view === "spam") {
        const folders = await getFolders(current);
        const folder = findFolderByType(folders, view === "trash" ? "TRASH" : "SPAM");
        if (!folder?.folderIdentifier) {
          ping(view === "trash" ? "没有回收站" : "没有垃圾箱");
          setMails([]);
          return;
        }
        const data = await listMail(current, { folderId: folder.folderIdentifier, amount: 80, tagsShowAll: true });
        list = sortMails((data && "mail" in data ? data.mail : []) || []).map((mail) => ({
          ...mail,
          sourceFolder: {
            folderIdentifier: folder.folderIdentifier || "",
            folderType: folder.attribute?.folderType || (view === "trash" ? "TRASH" : "SPAM"),
            folderName: folder.attribute?.folderName || (view === "trash" ? "回收站" : "垃圾邮件"),
          },
        }));
      } else {
        const data = await listMail(current, { amount: 80, tagsShowAll: true });
        list = sortMails((data && "mail" in data ? data.mail : []) || []);
      }
      if (!q.trim()) {
        await db.mailLists.put({ email: account.email, folder: folderKey, mails: list, updatedAt: Date.now() });
      }
      setMails(list);
      if (keepId) {
        const found = list.find((mail) => mailId(mail) === keepId) || null;
        setSelectedMail(found);
        if (found) {
          setHtml(keepHtml);
          setPreview(keepPreview);
        } else {
          setHtml("");
          setPreview("");
        }
      }
      if (!opts?.quiet) ping(`取到 ${list.length} 封`);
    } catch (error) {
      ping(error instanceof Error ? error.message : "收信失败");
    } finally {
      setBusy(null);
    }
  }

  async function fetchAll() {
    const targets = pickedAccounts.length ? pickedAccounts : [];
    if (!targets.length) {
      ping("先勾选要收信的账号");
      return;
    }
    const total = targets.length;
    progressRef.current = 0;
    setBusy({ text: `批量收信 0/${total}`, pane: "global", current: 0, total });
    await mapPool(targets, 3, async (account) => {
      try {
        const fresh = await db.accounts.get(account.email);
        if (!fresh?.accessToken) await loginAccount(account);
        const current = (await db.accounts.get(account.email)) || account;
        await listMail(current, { amount: 25, tagsShowAll: true });
      } catch {
        /* keep going */
      } finally {
        progressRef.current += 1;
        const current = progressRef.current;
        setBusy({ text: `批量收信 ${current}/${total}`, pane: "global", current, total });
      }
    });
    setBusy(null);
    if (selectedEmail) {
      const cached = await db.mailLists.get([selectedEmail, cacheFolderOf(box)]);
      if (cached) setMails(sortMails((cached.mails as MailMessage[]) || []));
    }
    ping("批量收信结束");
  }

  async function openMail(mail: MailMessage) {
    if (!selected || busy) return;
    const id = mailId(mail);
    const shouldRead = !isSentMail(mail) && mail.attribute?.read === false;
    setSelectedMail(mail);
    const cachedBody = await db.mailBodies.get([selected.email, id]);
    if (cachedBody?.html) {
      setHtml(cachedBody.html);
      setPreview(cachedBody.preview || "");
        if (shouldRead) {
          void getBody(selected, id, true).catch(() => undefined);
          patchRead(id);
        }
      return;
    }
    setHtml("");
    setPreview("");
    setBusy({ text: "正在读取正文", pane: "detail" });
    try {
      const [body, previews] = await Promise.all([
        getBody(selected, id, shouldRead),
        getPreview(selected, [id]).catch(() => []),
      ]);
      let nextHtml = body;
      const files = mail.attachments?.attachment || [];
      const inlineFiles = files.filter((item) => item.inline || item.contentId);
      if (inlineFiles.length) {
        const loaded = await Promise.all(
          inlineFiles.map(async (item) => {
            const aid = attachmentId(item);
            if (!aid) return null;
            try {
              const file = await downloadAttachment(selected, id, aid);
              return { item, file };
            } catch {
              return null;
            }
          }),
        );
        for (const row of loaded) {
          if (!row?.file.base64data) continue;
          const dataUrl = `data:${row.file.contentType};base64,${row.file.base64data}`;
          nextHtml = rewriteCid(nextHtml, row.item.contentId || "", dataUrl);
        }
      }
      setHtml(nextHtml);
      setPreview(previews[0]?.preview || "");
      await db.mailBodies.put({
        email: selected.email,
        mailId: id,
        html: nextHtml,
        preview: previews[0]?.preview || "",
        updatedAt: Date.now(),
      });
      if (shouldRead) patchRead(id);
    } catch (error) {
      ping(error instanceof Error ? error.message : "读取失败");
    } finally {
      setBusy(null);
    }
  }

  async function saveAttachment(item: MailAttachment) {
    if (!selected || !selectedMail) return;
    const aid = attachmentId(item);
    if (!aid) return;
    try {
      const file = await downloadAttachment(selected, mailId(selectedMail), aid);
      const bytes = Uint8Array.from(atob(file.base64data), (ch) => ch.charCodeAt(0));
      const blob = new Blob([bytes], { type: file.contentType });
      const url = URL.createObjectURL(blob);
      const link = document.createElement("a");
      link.href = url;
      link.download = item.filename || file.filename;
      link.click();
      URL.revokeObjectURL(url);
    } catch (error) {
      ping(error instanceof Error ? error.message : "下载失败");
    }
  }

  function switchBox(next: MailBox) {
    if (next === box) return;
    const needFetch = next === "trash" || next === "spam" || box === "trash" || box === "spam";
    setBox(next);
    if (needFetch && selected) void fetchMail(selected, query, { box: next });
    else if (selected) void fetchMail(selected, query, { box: next });
  }

  async function act(kind: "read" | "unread" | "star" | "unstar" | "spam" | "trash" | "delete" | "inbox") {
    if (!selected) {
      ping("先选择账号");
      return;
    }
    if (busy) return;
    const ids = [...selectedIds];
    if (!ids.length && selectedMail) ids.push(mailId(selectedMail));
    if (!ids.length) {
      ping("先勾选或打开一封邮件");
      return;
    }
    const confirms: Partial<Record<typeof kind, string>> = {
      trash: `把 ${ids.length} 封移入回收站？`,
      spam: `把 ${ids.length} 封标为垃圾邮件？`,
      delete: `永久删除 ${ids.length} 封？不可恢复`,
    };
    if (confirms[kind] && !window.confirm(confirms[kind])) return;
    const keep = kind === "read" || kind === "unread" || kind === "star" || kind === "unstar";
    setBusy({ text: "正在处理邮件", pane: "mail" });
    try {
      await runAction(kind, selected, ids);
      await fetchMail(selected, query, { keepSelection: keep, box, quiet: true, force: true });
      setSelectedIds(new Set());
      const done: Record<typeof kind, string> = {
        read: "已标为已读",
        unread: "已标为未读",
        star: "已加星标",
        unstar: "已取消星标",
        spam: "已标为垃圾邮件",
        trash: "已移入回收站",
        delete: "已永久删除",
        inbox: "已移回收件箱",
      };
      ping(done[kind]);
    } catch (error) {
      ping(error instanceof Error ? error.message : "操作失败");
      setBusy(null);
    }
  }

  async function copyCode() {
    if (!code) return;
    await navigator.clipboard.writeText(code);
    ping("验证码已复制");
  }

  async function exportAccounts() {
    const rows = pickedAccounts.length ? pickedAccounts : accounts;
    if (!rows.length) {
      ping("没有可导出的账号");
      return;
    }
    const lines = rows.map((a) => `${a.email}----${a.password}`).join("\n");
    await navigator.clipboard.writeText(lines);
    ping(`已复制 ${rows.length} 个账号`);
  }

  async function removeAccounts(emails: string[]) {
    const unique = [...new Set(emails.filter(Boolean))];
    if (!unique.length) return;
    if (!window.confirm(`删除 ${unique.length} 个账号？仅从本机移除`)) return;
    setAccountMenu(null);
    await db.accounts.bulkDelete(unique);
    await db.mailLists.where("email").anyOf(unique).delete();
    await db.mailBodies.where("email").anyOf(unique).delete();
    setPickedEmails((prev) => {
      const next = new Set(prev);
      unique.forEach((email) => next.delete(email));
      return next;
    });
    if (selectedEmail && unique.includes(selectedEmail)) {
      setSelectedEmail("");
      setMails([]);
      setSelectedMail(null);
      setHtml("");
      setPreview("");
    }
    ping(`已删除 ${unique.length} 个账号`);
  }

  return (
    <div className="relative h-full min-h-0">
      <div className="grain" />
      <div className="grid h-full min-h-0 grid-rows-[56px_1fr]">
        <header className="flex h-12 items-center justify-between border-b border-[var(--line)] px-4">
          <div className="flex items-center gap-3">
            <BrandMark size={22} />
            <div className="stamp text-[13px] text-[var(--gold)]">mail.com</div>
            <div className="h-4 w-px bg-[var(--line)]" />
            <div className="ticket text-[13px] text-[var(--mute)]">批量邮箱管理 · 账号仅保存在本机</div>
          </div>
          <div className="flex items-center gap-2">
            {busy && (
              <div className="mr-2 flex items-center gap-2 ticket text-[13px] text-[var(--gold)]">
                <LoaderCircle size={14} className="spin" />
                <span>{busy.text}</span>
              </div>
            )}
            <a
              href="https://github.com/yewenyell-lang/mail.com.free.manager"
              target="_blank"
              rel="noreferrer"
              title="开源地址"
              className="inline-flex h-8 items-center gap-1.5 rounded-[5px] border border-[var(--line)] px-3 text-[13px] text-[var(--mute)] hover:border-[var(--gold)] hover:text-[var(--gold)]"
            >
              <Github size={14} />
              GitHub
            </a>
            <TopBtn
              icon={themePref === "light" ? <Sun size={14} /> : themePref === "dark" ? <Moon size={14} /> : <Monitor size={14} />}
              onClick={() => setThemePref(cycleTheme(themePref))}
            >
              {themeLabel(themePref)}
            </TopBtn>
            <TopBtn icon={<Upload size={14} />} disabled={!!busy} onClick={() => setImportOpen(true)}>
              导入
            </TopBtn>
            {pickedAccounts.length > 0 && (
              <>
                <TopBtn icon={<LogIn size={14} />} disabled={!!busy} onClick={loginAll}>
                  批量登录 {pickedAccounts.length}
                </TopBtn>
                <TopBtn icon={<Inbox size={14} />} disabled={!!busy} onClick={fetchAll}>
                  批量收信 {pickedAccounts.length}
                </TopBtn>
                <TopBtn disabled={!!busy} onClick={exportAccounts}>
                  导出 {pickedAccounts.length}
                </TopBtn>
                <TopBtn disabled={!!busy} onClick={() => void removeAccounts(pickedAccounts.map((item) => item.email))}>
                  删除 {pickedAccounts.length}
                </TopBtn>
              </>
            )}
            <button
              disabled={!!busy}
              onClick={() => {
                if (!accounts.length) {
                  ping("先导入账号");
                  return;
                }
                setComposeOpen("send");
              }}
              className="inline-flex h-8 items-center gap-1.5 rounded-[5px] bg-[var(--gold-btn)] px-3 text-[13px] font-medium text-[var(--on-gold)] disabled:opacity-40"
            >
              <MailPlus size={14} />
              写信
            </button>
          </div>
        </header>

        <div className="grid min-h-0 grid-cols-[280px_minmax(360px,1.15fr)_minmax(480px,1.5fr)]">
          <section className="flex min-h-0 flex-col overflow-hidden border-r border-[var(--line)]">
            <PaneHead
              title="账号"
              extra={`${pickedAccounts.length} / ${accounts.length}`}
              action={
                <input
                  value={accountFilter}
                  onChange={(e) => setAccountFilter(e.target.value)}
                  placeholder="筛选邮箱"
                  className="h-8 w-36 rounded-[5px] bg-transparent ticket text-[13px] outline-none placeholder:text-[var(--mute)]"
                />
              }
            />
            <div className="flex items-center gap-2 border-b border-[var(--line)] px-4 py-2 text-[12px] text-[var(--mute)]">
              <label className="inline-flex items-center gap-1">
                <input
                  type="checkbox"
                  checked={filteredAccounts.length > 0 && filteredAccounts.every((account) => pickedEmails.has(account.email))}
                  onChange={() => {
                    const emails = filteredAccounts.map((account) => account.email);
                    const allOn = emails.length > 0 && emails.every((email) => pickedEmails.has(email));
                    const next = new Set(pickedEmails);
                    if (allOn) emails.forEach((email) => next.delete(email));
                    else emails.forEach((email) => next.add(email));
                    setPickedEmails(next);
                  }}
                />
                全选当前筛选
              </label>
            </div>
            <div className="min-h-0 flex-1 overflow-auto">
              {filteredAccounts.map((account) => {
                const active = account.email === selectedEmail;
                const rowBusy = account.status === "busy";
                return (
                  <div
                    key={account.email}
                    onContextMenu={(e) => {
                      e.preventDefault();
                      setAccountMenu({ email: account.email, x: e.clientX, y: e.clientY });
                    }}
                    className={`mx-1.5 my-0.5 flex w-[calc(100%-12px)] items-start gap-2.5 px-2.5 py-2.5 ${active ? "row-card bg-[var(--ink-3)]" : "row-card hover:bg-[var(--ink-2)]"}`}
                  >
                    <input
                      type="checkbox"
                      className="mt-1"
                      checked={pickedEmails.has(account.email)}
                      onChange={() => {
                        const next = new Set(pickedEmails);
                        if (next.has(account.email)) next.delete(account.email);
                        else next.add(account.email);
                        setPickedEmails(next);
                      }}
                    />
                    <button
                      disabled={!!busy}
                      onClick={() => fetchMail(account, "")}
                      className="min-w-0 flex-1 text-left disabled:opacity-60"
                    >
                      <div className="flex items-start gap-3">
                        {rowBusy ? <span className="spinner spinner-sm mt-1" /> : <StatusDot status={account.status} />}
                        <div className="min-w-0 flex-1">
                          <div className="ticket truncate text-[13px]">{account.email}</div>
                          <div className="mt-1 text-[12px] text-[var(--mute)]">
                            {rowBusy ? "处理中…" : account.status === "ok" ? "已登录" : account.status === "fail" ? account.lastError : "未登录"}
                          </div>
                        </div>
                      </div>
                    </button>
                    <button
                      title={hasUsableSession(account) ? "刷新邮件" : "登录"}
                      className="mt-0.5 rounded-[5px] p-1 text-[var(--mute)] opacity-55 hover:bg-[var(--ink-3)] hover:text-[var(--gold)] hover:opacity-100"
                      disabled={!!busy}
                      onClick={() => void loginOne(account)}
                    >
                      <RefreshCw size={13} className={rowBusy ? "spin" : ""} />
                    </button>
                  </div>
                );
              })}
              {!accounts.length && (
                <div className="px-4 py-10 text-sm text-[var(--mute)]">导入 `email:password` 或 `email----password`</div>
              )}
            </div>
          </section>

          <section className="flex min-h-0 flex-col overflow-hidden border-r border-[var(--line)]">
            <PaneHead
              title="邮件"
              extra={selected ? (box === "inbox" || box === "all" ? `${unread} 未读 / ${visibleMails.length}` : `${visibleMails.length} 封`) : "—"}
              action={
                <div className="flex items-center gap-2">
                  <Search size={13} className="text-[var(--mute)]" />
                  <input
                    value={query}
                    onChange={(e) => setQuery(e.target.value)}
                    onKeyDown={(e) => {
                      if (e.key === "Enter" && selected) void fetchMail(selected, query, { force: true });
                    }}
                    placeholder="搜索发件人/主题"
                    className="h-8 w-40 rounded-[5px] bg-transparent ticket text-[13px] outline-none placeholder:text-[var(--mute)]"
                  />
                </div>
              }
            />
            <div className="flex items-center justify-between gap-2 border-b border-[var(--line)] px-3 py-2">
              <div className="tab-strip">
                <Mini active={box === "inbox"} onClick={() => switchBox("inbox")}>收件</Mini>
                <Mini active={box === "sent"} onClick={() => switchBox("sent")}>已发送</Mini>
                <Mini active={box === "trash"} onClick={() => switchBox("trash")}>回收站</Mini>
                <Mini active={box === "spam"} onClick={() => switchBox("spam")}>垃圾箱</Mini>
                <Mini active={box === "all"} onClick={() => switchBox("all")}>全部</Mini>
              </div>
              <Mini
                disabled={!!busy || !selected}
                onClick={() => selected && void fetchMail(selected, query, { force: true })}
              >
                刷新
              </Mini>
            </div>
            <div className="flex flex-wrap items-center gap-1 border-b border-[var(--line)] px-3 py-2 text-[12px]">
              <label className="mr-1 inline-flex items-center gap-1.5 text-[var(--mute)]">
                <input
                  type="checkbox"
                  checked={visibleMails.length > 0 && visibleMails.every((mail) => selectedIds.has(mailId(mail)))}
                  onChange={() => {
                    const ids = visibleMails.map(mailId);
                    const allOn = ids.length > 0 && ids.every((id) => selectedIds.has(id));
                    setSelectedIds(allOn ? new Set() : new Set(ids));
                  }}
                />
                全选
              </label>
              {(selectedIds.size > 0 || selectedMail) && (
                box === "trash" ? (
                  <>
                    <Mini disabled={!!busy} onClick={() => void act("inbox")}>移回收件箱</Mini>
                    <Mini disabled={!!busy} onClick={() => void act("delete")}>永久删除</Mini>
                  </>
                ) : box === "spam" ? (
                  <>
                    <Mini disabled={!!busy} onClick={() => void act("inbox")}>移回收件箱</Mini>
                    <Mini disabled={!!busy} onClick={() => void act("trash")}>移入回收站</Mini>
                    <Mini disabled={!!busy} onClick={() => void act("delete")}>永久删除</Mini>
                  </>
                ) : (
                  <>
                    <Mini disabled={!!busy} onClick={() => void act("read")}>已读</Mini>
                    <Mini disabled={!!busy} onClick={() => void act("unread")}>未读</Mini>
                    <Mini
                      disabled={!!busy}
                      onClick={() => {
                        const targets = selectedIds.size
                          ? visibleMails.filter((mail) => selectedIds.has(mailId(mail)))
                          : selectedMail
                            ? [selectedMail]
                            : [];
                        const starred = targets.some((mail) => mail.attribute?.flagged);
                        void act(starred ? "unstar" : "star");
                      }}
                    >
                      星标
                    </Mini>
                    <Mini disabled={!!busy} onClick={() => void act("trash")}>回收站</Mini>
                    <Mini disabled={!!busy} onClick={() => void act("spam")}>垃圾箱</Mini>
                    <Mini disabled={!!busy} onClick={() => void act("delete")}>删除</Mini>
                  </>
                )
              )}
            </div>
            <div className="relative min-h-0 flex-1 overflow-auto">
              {busy?.pane === "mail" && <LoadingCover text={busy.text} />}
              {busy?.pane === "mail" && !mails.length && <MailSkeleton />}
              {visibleMails.map((mail) => {
                const id = mailId(mail);
                const active = mailId(selectedMail || {}) === id;
                const sent = isSentMail(mail);
                const unreadMail = !sent && mail.attribute?.read === false;
                const starred = mail.attribute?.flagged === true;
                return (
                  <div
                    key={id}
                    className={`mx-1.5 my-0.5 flex cursor-pointer gap-3 px-2.5 py-2.5 ${active ? "row-card bg-[var(--ink-3)]" : "row-card hover:bg-[var(--ink-2)]"}`}
                    onClick={() => void openMail(mail)}
                  >
                    <input
                      type="checkbox"
                      checked={selectedIds.has(id)}
                      onChange={(e) => {
                        e.stopPropagation();
                        const next = new Set(selectedIds);
                        if (next.has(id)) next.delete(id);
                        else next.add(id);
                        setSelectedIds(next);
                      }}
                      onClick={(e) => e.stopPropagation()}
                    />
                    <div className="min-w-0 flex-1">
                      <div className="flex items-center gap-2">
                        {unreadMail && <span className="unread-dot" />}
                        <span className={`ticket shrink-0 text-[11px] ${sent ? "text-[var(--gold)]" : isTrashMail(mail) || isSpamMail(mail) ? "text-[var(--seal)]" : "text-[var(--mute)]"}`}>
                          {sent ? "发" : isTrashMail(mail) ? "废" : isSpamMail(mail) ? "垃" : "收"}
                        </span>
                        {starred && <span className="ticket shrink-0 text-[10px] text-[var(--gold)]">★</span>}
                        <div className={`truncate text-[14px] ${unreadMail ? "font-medium text-[var(--paper)]" : "text-[var(--mute)]"}`}>
                          {mail.mailHeader?.subject || "(无主题)"}
                        </div>
                      </div>
                      <div className="mt-1 flex justify-between ticket text-[12px] text-[var(--mute)]">
                        <span className="truncate pr-3">{counterpart(mail)}</span>
                        <span>{fmtTime(mail.mailHeader?.date)}</span>
                      </div>
                    </div>
                  </div>
                );
              })}
              {selected && !visibleMails.length && busy?.pane !== "mail" && (
                <div className="px-4 py-10 text-sm text-[var(--mute)]">
                  {box === "sent" ? "没有已发送邮件" : box === "trash" ? "回收站为空" : box === "spam" ? "垃圾箱为空" : "此箱暂无邮件"}
                </div>
              )}
            </div>
          </section>

          <section className="min-h-0 overflow-hidden">
            <PaneHead
              title="详情"
              extra={selectedMail ? (isSentMail(selectedMail) ? "已发送" : selectedMail.sourceFolder?.folderName || selectedMail.sourceFolder?.folderType || "收件") : ""}
              action={
                <div className="flex gap-1">
                  <Mini disabled={!!busy || !selectedMail || isSentMail(selectedMail)} onClick={() => setComposeOpen("reply")}>回复</Mini>
                  <Mini disabled={!!busy} onClick={() => setComposeOpen("forward")}>转发</Mini>
                </div>
              }
            />
            {selectedMail ? (
              <div className="grid h-[calc(100%-44px)] min-h-0 grid-rows-[auto_1fr]">
                <div className="border-b border-[var(--line)] px-5 py-4">
                  <div className="text-[18px] font-medium leading-snug">{selectedMail.mailHeader?.subject || "(无主题)"}</div>
                  <div className="mt-2 ticket text-[13px] text-[var(--mute)]">{counterpart(selectedMail)}</div>
                  {code && (
                    <button
                      onClick={() => void copyCode()}
                      className="mt-3 inline-flex items-center gap-2 rounded-[5px] border border-[var(--gold)] px-3 py-1.5 text-[var(--gold)]"
                    >
                      <Copy size={13} />
                      <span className="ticket text-[15px] tracking-[0.12em]">{code}</span>
                    </button>
                  )}
                  {!!selectedMail.attachments?.attachment?.length && (
                    <div className="mt-3 flex flex-wrap gap-2">
                      {selectedMail.attachments.attachment.map((item, index) => (
                        <button
                          key={item.attachmentURI || `${item.filename}-${index}`}
                          className="rounded-[5px] border border-[var(--line)] px-2 py-1 text-[12px] hover:border-[var(--gold)]"
                          onClick={() => void saveAttachment(item)}
                        >
                          {item.inline ? "图片" : "附件"} · {item.filename || "未命名"}
                        </button>
                      ))}
                    </div>
                  )}
                </div>
                <div className="letter-stage relative min-h-0 overflow-auto p-6">
                  {busy?.pane === "detail" && <LoadingCover text={busy.text} />}
                  {busy?.pane === "detail" ? (
                    <div className="letter-sheet space-y-3 p-8">
                      <div className="h-4 w-5/6 bg-[var(--paper-2)]" />
                      <div className="h-4 w-full bg-[var(--paper-2)]" />
                      <div className="h-4 w-2/3 bg-[var(--paper-2)]" />
                    </div>
                  ) : (
                    <div
                      className="letter-sheet prose max-w-none p-8 text-[16px] leading-7"
                      dangerouslySetInnerHTML={{ __html: sanitizeBody(html || preview) }}
                    />
                  )}
                </div>
              </div>
            ) : (
              <div className="letter-stage flex h-[calc(100%-44px)] items-center justify-center text-[var(--mute)]">选择一封邮件</div>
            )}
          </section>
        </div>
      </div>

      {accountMenu && (
        <>
          <div className="fixed inset-0 z-40" onClick={() => setAccountMenu(null)} onContextMenu={(e) => { e.preventDefault(); setAccountMenu(null); }} />
          <div
            className="fixed z-50 min-w-40 rounded-lg border border-[var(--line)] bg-[var(--ink-2)] py-1 text-[13px] shadow-xl"
            style={{ left: Math.min(accountMenu.x, window.innerWidth - 180), top: Math.min(accountMenu.y, window.innerHeight - 180) }}
          >
            {(() => {
              const target = accounts.find((item) => item.email === accountMenu.email);
              if (!target) return null;
              return (
                <>
                  <button className="block w-full px-3 py-1.5 text-left hover:bg-[var(--ink-3)]" onClick={() => void copyAccount(target)}>
                    复制邮箱
                  </button>
                  <button className="block w-full px-3 py-1.5 text-left hover:bg-[var(--ink-3)]" onClick={() => void copyAccount(target, true)}>
                    复制邮箱和密码
                  </button>
                  <button
                    className="block w-full px-3 py-1.5 text-left hover:bg-[var(--ink-3)]"
                    disabled={!!busy}
                    onClick={() => void reloginOne(target)}
                  >
                    重新登录
                  </button>
                  <button
                    className="block w-full px-3 py-1.5 text-left text-[var(--seal)] hover:bg-[var(--ink-3)]"
                    disabled={!!busy}
                    onClick={() => void removeAccounts([target.email])}
                  >
                    删除账号
                  </button>
                </>
              );
            })()}
          </div>
        </>
      )}
      {busy?.pane === "global" && (
        <div className="fixed inset-0 z-20 flex items-center justify-center bg-black/45">
          <div className="flex min-w-[280px] flex-col items-center gap-4 border border-[var(--gold)] bg-[var(--ink)] px-8 py-6">
            <span className="spinner" />
            <div className="ticket text-[13px] text-[var(--gold)]">{busy.text}</div>
            {!!busy.total && (
              <div className="h-1 w-full bg-[var(--ink-3)]">
                <div
                  className="h-full bg-[var(--gold)] transition-all"
                  style={{ width: `${Math.round(((busy.current || 0) / busy.total) * 100)}%` }}
                />
              </div>
            )}
          </div>
        </div>
      )}
      {toast && (
        <div
          className={`toast-banner fixed inset-x-0 top-14 z-[80] mx-auto w-fit rounded-md px-4 py-2.5 text-sm font-medium shadow-[0_10px_30px_rgba(0,0,0,0.45)] ${toast.kind === "error" ? "bg-[var(--seal)] text-white" : "bg-[var(--gold-btn)] text-[var(--on-gold)]"}`}
        >
          {toast.text}
        </div>
      )}

      {importOpen && (
        <Modal title="导入账号" onClose={() => setImportOpen(false)}>
          <textarea
            value={importText}
            onChange={(e) => setImportText(e.target.value)}
            className="h-56 w-full bg-[var(--ink)] p-3 ticket text-[13px] outline-none"
            placeholder={"email:password\nemail----password"}
          />
          <div className="mt-3 flex justify-end gap-2">
            <Mini onClick={() => setImportOpen(false)}>取消</Mini>
            <button className="rounded-[5px] bg-[var(--gold-btn)] px-4 py-2 font-medium text-[var(--on-gold)]" onClick={() => void importNow()}>
              写入本机
            </button>
          </div>
        </Modal>
      )}

      {composeOpen && composeAccount && (
        <Suspense
          fallback={
            <div className="fixed inset-0 z-[70] flex justify-end bg-black/45">
              <div className="flex h-full w-[min(920px,92vw)] items-center justify-center border-l border-[var(--line)] bg-[var(--ink-2)]">
                <span className="spinner" />
              </div>
            </div>
          }
        >
          <Compose
            mode={composeOpen}
            account={composeAccount}
            original={selectedMail}
            onClose={() => setComposeOpen(null)}
            onDone={(msg) => {
              setComposeOpen(null);
              ping(msg);
            }}
          />
        </Suspense>
      )}
    </div>
  );
}

function TopBtn({
  children,
  onClick,
  icon,
  disabled,
}: {
  children: ReactNode;
  onClick?: () => void;
  icon?: ReactNode;
  disabled?: boolean;
}) {
  return (
    <button
      disabled={disabled}
      onClick={onClick}
      className="inline-flex h-8 items-center gap-1.5 rounded-[5px] border border-[var(--line)] px-3 text-[13px] hover:border-[var(--gold)] hover:text-[var(--gold)] disabled:cursor-not-allowed disabled:opacity-40"
    >
      {icon}
      {children}
    </button>
  );
}

function LoadingCover({ text, light }: { text: string; light?: boolean }) {
  return (
    <div
      className={`absolute inset-0 z-10 flex flex-col items-center justify-center gap-3 ${light ? "bg-[var(--cover-light)]" : "bg-[var(--cover)]"}`}
    >
      <span className="spinner" />
      <div className={`ticket text-[13px] ${light ? "text-[#6b5c3a]" : "text-[var(--gold)]"}`}>{text}</div>
    </div>
  );
}

function MailSkeleton() {
  return (
    <div className="space-y-0">
      {Array.from({ length: 7 }).map((_, i) => (
        <div key={i} className="border-b border-[var(--line)] px-4 py-3">
          <div className="h-3 w-3/5 bg-[var(--ink-3)]" />
          <div className="mt-2 h-2 w-2/5 bg-[var(--ink-3)]" />
        </div>
      ))}
    </div>
  );
}

function Mini({
  children,
  onClick,
  disabled,
  active,
}: {
  children: ReactNode;
  onClick?: () => void;
  disabled?: boolean;
  active?: boolean;
}) {
  return (
    <button
      disabled={disabled}
      onClick={onClick}
      className={`rounded-[5px] px-2.5 py-1 text-[12px] disabled:cursor-not-allowed disabled:opacity-40 ${active ? "bg-[var(--ink-3)] text-[var(--gold)]" : "text-[var(--mute)] hover:bg-[var(--ink-3)] hover:text-[var(--paper)]"}`}
    >
      {children}
    </button>
  );
}

function PaneHead({ title, extra, action }: { title: string; extra?: string; action?: ReactNode }) {
  return (
    <div className="flex h-11 items-center justify-between border-b border-[var(--line)] px-4">
      <div className="flex items-baseline gap-2">
        <div className="stamp text-[12px] text-[var(--mute)]">{title}</div>
        <div className="ticket text-[12px] text-[var(--gold)]">{extra}</div>
      </div>
      {action}
    </div>
  );
}

function StatusDot({ status }: { status: AccountRow["status"] }) {
  const color =
    status === "ok" ? "var(--ok)" : status === "fail" ? "var(--seal)" : status === "busy" ? "var(--gold)" : "var(--mute)";
  return <span className="mt-1.5 inline-block h-2 w-2 rounded-full" style={{ background: color }} />;
}

function Modal({ title, onClose, children }: { title: string; onClose: () => void; children: ReactNode }) {
  return (
    <div className="fixed inset-0 z-30 flex items-center justify-center bg-black/55 p-6">
      <div className="w-full max-w-xl rounded-lg border border-[var(--line)] bg-[var(--ink-2)] p-5">
        <div className="mb-4 flex items-center justify-between">
          <div className="stamp text-[13px] text-[var(--gold)]">{title}</div>
          <button onClick={onClose} className="text-[var(--mute)]">
            关闭
          </button>
        </div>
        {children}
      </div>
    </div>
  );
}


