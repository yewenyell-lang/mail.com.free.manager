const labeled =
  /(?:验证码|校验码|code|otp|verify|verification)[^\d]{0,24}(\d{4,8})/i;
const bare = /\b(\d{6})\b/;

export function extractCode(...chunks: Array<string | undefined | null>): string | null {
  const text = chunks.filter(Boolean).join("\n");
  if (!text) return null;
  const labeledMatch = text.match(labeled);
  if (labeledMatch?.[1]) return labeledMatch[1];
  const bareMatch = text.match(bare);
  return bareMatch?.[1] ?? null;
}
