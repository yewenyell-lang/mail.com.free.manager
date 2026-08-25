export type Session = {
  accessToken?: string;
  refreshToken?: string;
  expiresAt?: number;
  accountEmail?: string;
};

export type MailHeader = {
  from?: string;
  to?: string[];
  cc?: string[];
  bcc?: string[];
  subject?: string;
  date?: number;
};

export type MailAttachment = {
  attachmentURI?: string;
  contentType?: string;
  contentId?: string;
  filename?: string;
  estimatedSize?: number;
  inline?: boolean;
};

export type MailMessage = {
  mailURI?: string;
  attribute?: {
    mailIdentifier?: string;
    folderIdentifier?: string;
    folderType?: string;
    read?: boolean;
    flagged?: boolean;
    hasDownloadableAttachments?: boolean;
  };
  mailHeader?: MailHeader;
  attachments?: { attachment?: MailAttachment[] };
  sourceFolder?: {
    folderIdentifier: string;
    folderType: string;
    folderName?: string;
  };
};

export type IncomingData = {
  mail: MailMessage[];
  totalCount: number;
  unreadCount: number;
  folders: Array<{ folderIdentifier: string; folderType: string; folderName?: string }>;
};

export type Folder = {
  folderIdentifier?: string;
  attribute?: {
    folderName?: string;
    folderFullname?: string;
    folderType?: string;
  };
  folders?: Folder[];
};

export type MailBox = "inbox" | "sent" | "trash" | "spam" | "all";
