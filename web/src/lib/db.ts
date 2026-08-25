import Dexie, { type EntityTable } from "dexie";

export type AccountStatus = "unknown" | "ok" | "fail" | "busy";

export type AccountRow = {
  email: string;
  password: string;
  accessToken?: string;
  refreshToken?: string;
  expiresAt?: number;
  status: AccountStatus;
  lastError?: string;
  lastLoginAt?: number;
  note?: string;
  tags: string[];
};

export type MailListRow = {
  email: string;
  folder: string;
  mails: unknown[];
  updatedAt: number;
};

export type MailBodyRow = {
  email: string;
  mailId: string;
  html: string;
  preview: string;
  updatedAt: number;
};

class DeskDB extends Dexie {
  accounts!: EntityTable<AccountRow, "email">;
  mailLists!: EntityTable<MailListRow, "email">;
  mailBodies!: EntityTable<MailBodyRow, "email">;

  constructor() {
    super("sorting-desk-2");
    this.version(1).stores({
      accounts: "email, status, lastLoginAt",
      mailLists: "[email+folder], email, updatedAt",
      mailBodies: "[email+mailId], email, updatedAt",
    });
  }
}

export const db = new DeskDB();

export function cacheFolderOf(box: string) {
  if (box === "trash") return "trash";
  if (box === "spam") return "spam";
  return "incoming";
}

export async function recoverAccountsIfEmpty() {
  try {
    await db.open();
    if ((await db.accounts.count()) > 0) return;
    const dumped = await readRawAccounts("sorting-desk");
    if (dumped.length) await db.accounts.bulkPut(dumped);
  } catch {
    /* keep empty */
  }
}

function readRawAccounts(name: string) {
  return new Promise<AccountRow[]>((resolve) => {
    const req = indexedDB.open(name);
    req.onerror = () => resolve([]);
    req.onsuccess = () => {
      const idb = req.result;
      if (!idb.objectStoreNames.contains("accounts")) {
        idb.close();
        resolve([]);
        return;
      }
      try {
        const tx = idb.transaction("accounts", "readonly");
        const all = tx.objectStore("accounts").getAll();
        all.onsuccess = () => {
          idb.close();
          resolve((all.result || []) as AccountRow[]);
        };
        all.onerror = () => {
          idb.close();
          resolve([]);
        };
      } catch {
        idb.close();
        resolve([]);
      }
    };
  });
}
