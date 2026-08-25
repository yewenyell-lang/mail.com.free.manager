import { Paperclip, X } from "lucide-react";
import { useEffect, useMemo, useRef, useState, type ComponentProps, type ReactNode } from "react";
import type { Editor as TinyEditor } from "tinymce";
import BundledEditor from "./editor/BundledEditor";
import { forwardMail, getAliases, getBody, replyMail, sendMail } from "./lib/api";
import type { AccountRow } from "./lib/db";
import type { MailMessage } from "./lib/types";

const MAX_ATTACH_BYTES = 25 * 1024 * 1024;

type Attachment = {
  contentType: string;
  filename: string;
  base64data: string;
  size: number;
  contentId?: string;
  inline?: boolean;
};

function mailId(mail: MailMessage) {
  return mail.attribute?.mailIdentifier || mail.mailURI || "";
}

function splitAddrs(value: string) {
  return value.split(/[,;]+/).map((item) => item.trim()).filter(Boolean);
}

function formatSize(bytes: number) {
  if (bytes < 1024) return `${bytes} B`;
  if (bytes < 1024 * 1024) return `${(bytes / 1024).toFixed(1)} KB`;
  return `${(bytes / (1024 * 1024)).toFixed(1)} MB`;
}

function readFile(file: File): Promise<Attachment> {
  return new Promise((resolve, reject) => {
    const reader = new FileReader();
    reader.onload = () => {
      const result = String(reader.result || "");
      const base64data = result.includes(",") ? result.slice(result.indexOf(",") + 1) : result;
      resolve({
        contentType: file.type || "application/octet-stream",
        filename: file.name,
        base64data,
        size: file.size,
      });
    };
    reader.onerror = () => reject(new Error(`读取失败: ${file.name}`));
    reader.readAsDataURL(file);
  });
}

function quoteOriginal(mail: MailMessage, html: string) {
  const from = mail.mailHeader?.from || "";
  const subject = mail.mailHeader?.subject || "";
  return `<p></p><hr /><p style="color:#666;font-size:12px">----- 原始邮件 -----</p><p style="color:#666;font-size:12px">发件人: ${escapeHtml(from)}</p><p style="color:#666;font-size:12px">主题: ${escapeHtml(subject)}</p><blockquote style="border-left:3px solid #d0c8b8;margin:0;padding:0 0 0 12px">${html || "<p></p>"}</blockquote>`;
}

function escapeHtml(value: string) {
  return value
    .replaceAll("&", "&amp;")
    .replaceAll("<", "&lt;")
    .replaceAll(">", "&gt;")
    .replaceAll('"', "&quot;");
}

function extOf(mime: string) {
  const subtype = mime.split("/")[1]?.split("+")[0] || "png";
  if (subtype === "jpeg") return "jpg";
  if (subtype === "svg+xml") return "svg";
  return subtype;
}

function extractInlineImages(html: string): { html: string; attachments: Attachment[] } {
  const images: Attachment[] = [];
  let index = 0;
  const next = html.replace(/src\s*=\s*(["'])data:(image\/[a-zA-Z0-9.+-]+);base64,([^"']+)\1/gi, (_full, _quote, mime, data) => {
    index += 1;
    const cid = `img${index}@sortingdesk`;
    const clean = String(data).replace(/\s+/g, "");
    images.push({
      contentType: mime,
      filename: `image${index}.${extOf(mime)}`,
      base64data: clean,
      size: Math.floor((clean.length * 3) / 4),
      contentId: cid,
      inline: true,
    });
    return `src="cid:${cid}"`;
  });
  return { html: next, attachments: images };
}

const editorInit: NonNullable<ComponentProps<typeof BundledEditor>["init"]> = {
  height: "100%",
  min_height: 280,
  menubar: "file edit view insert format tools table",
  plugins: [
    "advlist",
    "anchor",
    "autolink",
    "charmap",
    "code",
    "directionality",
    "fullscreen",
    "image",
    "insertdatetime",
    "link",
    "lists",
    "nonbreaking",
    "preview",
    "quickbars",
    "searchreplace",
    "table",
    "visualblocks",
    "wordcount",
  ],
  toolbar:
    "undo redo | fontfamily fontsize | bold italic underline strikethrough | forecolor backcolor | alignleft aligncenter alignright alignjustify | bullist numlist outdent indent | link image table | blockquote removeformat | code fullscreen",
  toolbar_mode: "sliding",
  font_family_formats:
    "微软雅黑=Microsoft YaHei, Microsoft YaHei UI, sans-serif;宋体=SimSun, serif;黑体=SimHei, sans-serif;Arial=Arial, Helvetica, sans-serif;Georgia=Georgia, serif;Tahoma=Tahoma, sans-serif;Verdana=Verdana, Geneva, sans-serif;Courier New=Courier New, Courier, monospace",
  font_size_formats: "10px 12px 14px 16px 18px 24px 32px",
  quickbars_selection_toolbar: "bold italic underline | forecolor | link",
  quickbars_insert_toolbar: false,
  branding: false,
  promotion: false,
  statusbar: true,
  skin: false,
  content_css: false,
  convert_urls: false,
  relative_urls: false,
  paste_data_images: true,
  automatic_uploads: true,
  images_file_types: "jpeg,jpg,png,gif,webp",
  images_upload_handler: (blobInfo) =>
    Promise.resolve(`data:${blobInfo.blob().type || "image/png"};base64,${blobInfo.base64()}`),
  content_style:
    "html { height: 100%; } body { min-height: 100%; font-family: 'Segoe UI', 'Microsoft YaHei UI', 'Microsoft YaHei', sans-serif; font-size: 14px; color: #1a1610; background: #f4efe4; line-height: 1.6; margin: 12px; }",
};

export default function Compose({
  mode,
  account,
  original,
  onClose,
  onDone,
}: {
  mode: "send" | "reply" | "forward";
  account: AccountRow;
  original: MailMessage | null;
  onClose: () => void;
  onDone: (msg: string) => void;
}) {
  const editorRef = useRef<TinyEditor | null>(null);
  const fileRef = useRef<HTMLInputElement>(null);
  const [to, setTo] = useState(mode === "reply" ? original?.mailHeader?.from || "" : "");
  const [cc, setCc] = useState(mode === "reply" ? (original?.mailHeader?.cc || []).join(", ") : "");
  const [bcc, setBcc] = useState("");
  const [showCc, setShowCc] = useState(Boolean(original?.mailHeader?.cc?.length) && mode === "reply");
  const [showBcc, setShowBcc] = useState(false);
  const [subject, setSubject] = useState(
    mode === "reply" ? `Re: ${original?.mailHeader?.subject || ""}` : mode === "forward" ? `Fwd: ${original?.mailHeader?.subject || ""}` : "",
  );
  const [from, setFrom] = useState(account.email);
  const [aliases, setAliases] = useState<string[]>([account.email]);
  const [priority, setPriority] = useState("3");
  const [sending, setSending] = useState(false);
  const [attachments, setAttachments] = useState<Attachment[]>([]);
  const [initialHtml, setInitialHtml] = useState(mode === "send" ? "<p></p>" : "");
  const [ready, setReady] = useState(mode === "send");
  const [error, setError] = useState("");

  const attachTotal = useMemo(() => attachments.reduce((sum, item) => sum + item.size, 0), [attachments]);

  useEffect(() => {
    function onKey(event: KeyboardEvent) {
      if (event.key === "Escape" && !sending) onClose();
    }
    window.addEventListener("keydown", onKey);
    return () => window.removeEventListener("keydown", onKey);
  }, [onClose, sending]);

  useEffect(() => {
    void getAliases(account)
      .then((list) => {
        const addresses = list.map((item) => item.address).filter(Boolean);
        if (addresses.length) {
          setAliases(addresses);
          const def = list.find((item) => item.defaultSenderAddress)?.address;
          if (def) setFrom(def);
        }
      })
      .catch(() => undefined);
  }, [account]);

  useEffect(() => {
    if (mode === "send" || !original) {
      setReady(true);
      return;
    }
    let cancelled = false;
    void getBody(account, mailId(original), false)
      .then((html) => {
        if (!cancelled) setInitialHtml(quoteOriginal(original, html));
      })
      .catch(() => {
        if (!cancelled) setInitialHtml(quoteOriginal(original, ""));
      })
      .finally(() => {
        if (!cancelled) setReady(true);
      });
    return () => {
      cancelled = true;
    };
  }, [account, mode, original]);

  async function addFiles(list: FileList | null) {
    if (!list?.length) return;
    setError("");
    try {
      const incoming = await Promise.all(Array.from(list).map(readFile));
      const next = [...attachments, ...incoming];
      const total = next.reduce((sum, item) => sum + item.size, 0);
      if (total > MAX_ATTACH_BYTES) {
        setError("附件合计超过 25 MB");
        return;
      }
      setAttachments(next);
    } catch (err) {
      setError(err instanceof Error ? err.message : "读取附件失败");
    }
  }

  async function submit() {
    const html = editorRef.current?.getContent({ format: "html" }) || "";
    const toList = splitAddrs(to);
    if (!toList.length) {
      setError("请填写收件人");
      return;
    }
    setSending(true);
    setError("");
    try {
      const inlined = extractInlineImages(html);
      const allAttachments = [...attachments, ...inlined.attachments];
      const total = allAttachments.reduce((sum, item) => sum + item.size, 0);
      if (total > MAX_ATTACH_BYTES) {
        setError("正文图片和附件合计超过 25 MB");
        return;
      }
      const body = inlined.html.includes("<html") ? inlined.html : `<html><body>${inlined.html || "<p></p>"}</body></html>`;
      const payload = {
        from,
        to: toList,
        cc: splitAddrs(cc),
        bcc: splitAddrs(bcc),
        subject,
        priority,
        htmlBody: body,
        attachments: allAttachments.map(({ contentType, filename, base64data, contentId, inline }) => ({
          contentType,
          filename,
          base64data,
          contentId,
          inline,
        })),
      };
      if (mode === "reply" && original) {
        await replyMail(account, { ...payload, originalMailId: mailId(original) });
      } else if (mode === "forward" && original) {
        await forwardMail(account, { ...payload, originalMailId: mailId(original) });
      } else {
        await sendMail(account, payload);
      }
      onDone("已发送");
    } catch (err) {
      setError(err instanceof Error ? err.message : "发送失败");
    } finally {
      setSending(false);
    }
  }

  const title = mode === "send" ? "撰写" : mode === "reply" ? "回复" : "转发";

  return (
    <div className="fixed inset-0 z-[70] flex justify-end">
      <button className="h-full flex-1 bg-black/45" aria-label="关闭撰写" onClick={onClose} />
      <aside className="compose-drawer flex h-full w-[min(920px,92vw)] flex-col border-l border-[var(--line)] bg-[var(--ink-2)]">
        <div className="flex shrink-0 items-center justify-between border-b border-[var(--line)] px-5 py-3">
          <div className="stamp text-[13px] text-[var(--gold)]">{title}</div>
          <div className="flex items-center gap-3 text-[12px] text-[var(--mute)]">
            {!showCc && (
              <button onClick={() => setShowCc(true)} className="hover:text-[var(--gold)]">
                抄送
              </button>
            )}
            {!showBcc && (
              <button onClick={() => setShowBcc(true)} className="hover:text-[var(--gold)]">
                密送
              </button>
            )}
            <button onClick={onClose} className="hover:text-[var(--paper)]">
              关闭
            </button>
          </div>
        </div>

        <div className="grid shrink-0 gap-2 px-5 pt-4">
          <Field label="发件人">
            <select value={from} onChange={(e) => setFrom(e.target.value)} className="field-input ticket">
              {aliases.map((alias) => (
                <option key={alias}>{alias}</option>
              ))}
            </select>
          </Field>
          <Field label="收件人">
            <input value={to} onChange={(e) => setTo(e.target.value)} placeholder="多个地址用逗号分隔" className="field-input ticket" />
          </Field>
          {showCc && (
            <Field label="抄送">
              <input value={cc} onChange={(e) => setCc(e.target.value)} className="field-input ticket" />
            </Field>
          )}
          {showBcc && (
            <Field label="密送">
              <input value={bcc} onChange={(e) => setBcc(e.target.value)} className="field-input ticket" />
            </Field>
          )}
          <Field label="主题">
            <input value={subject} onChange={(e) => setSubject(e.target.value)} className="field-input" />
          </Field>
          <Field label="优先级">
            <select value={priority} onChange={(e) => setPriority(e.target.value)} className="field-input w-36">
              <option value="1">高</option>
              <option value="3">普通</option>
              <option value="5">低</option>
            </select>
          </Field>
        </div>

        <div className="compose-editor min-h-0 flex-1 px-5 pt-3">
          {ready ? (
            <BundledEditor
              onInit={(_evt, editor) => {
                editorRef.current = editor;
              }}
              initialValue={initialHtml}
              init={editorInit}
            />
          ) : (
            <div className="flex h-full items-center justify-center gap-3 border border-[var(--line)] bg-[var(--ink)]">
              <span className="spinner" />
              <span className="ticket text-[13px] text-[var(--gold)]">正在载入原信…</span>
            </div>
          )}
        </div>

        <div className="shrink-0 border-t border-[var(--line)] bg-[var(--ink)] px-5 py-3">
          <div className="flex items-center justify-between">
            <button
              className="inline-flex items-center gap-2 text-[13px] hover:text-[var(--gold)]"
              onClick={() => fileRef.current?.click()}
            >
              <Paperclip size={14} />
              添加附件
            </button>
            <div className="ticket text-[12px] text-[var(--mute)]">
              {formatSize(attachTotal)} / 25 MB
            </div>
          </div>
          <input
            ref={fileRef}
            type="file"
            multiple
            className="hidden"
            onChange={(e) => {
              void addFiles(e.target.files);
              e.target.value = "";
            }}
          />
          {!!attachments.length && (
            <div className="mt-2 grid max-h-28 gap-1 overflow-auto">
              {attachments.map((item, index) => (
                <div key={`${item.filename}-${index}`} className="flex items-center justify-between bg-[var(--ink-2)] px-2 py-1.5 text-[12px]">
                  <span className="truncate pr-3">
                    {item.filename}
                    <span className="ml-2 text-[var(--mute)]">{formatSize(item.size)}</span>
                  </span>
                  <button
                    className="text-[var(--mute)] hover:text-[var(--seal)]"
                    onClick={() => setAttachments(attachments.filter((_, i) => i !== index))}
                  >
                    <X size={14} />
                  </button>
                </div>
              ))}
            </div>
          )}
          {error && <div className="mt-2 text-[13px] text-[var(--seal)]">{error}</div>}
        </div>

        <div className="flex shrink-0 justify-end gap-2 border-t border-[var(--line)] px-5 py-3">
          <button onClick={onClose} className="border border-[var(--line)] px-3 py-1.5 text-[12px] hover:border-[var(--gold)]">
            取消
          </button>
          <button
            disabled={sending || !ready}
            className="inline-flex items-center gap-2 bg-[var(--gold)] px-4 py-2 text-black disabled:opacity-70"
            onClick={() => void submit()}
          >
            {sending && <span className="spinner spinner-sm spinner-ink" />}
            {sending ? "发送中…" : "发送"}
          </button>
        </div>
      </aside>
    </div>
  );
}

function Field({ label, children }: { label: string; children: ReactNode }) {
  return (
    <label className="grid grid-cols-[64px_1fr] items-center gap-3 text-[13px]">
      <span className="text-[var(--mute)]">{label}</span>
      {children}
    </label>
  );
}
