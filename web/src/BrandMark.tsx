export default function BrandMark({ size = 22 }: { size?: number }) {
  return (
    <svg xmlns="http://www.w3.org/2000/svg" width={size} height={size} viewBox="0 0 32 32" aria-hidden>
      <rect width="32" height="32" rx="6" fill="#12110e" />
      <rect x="5" y="7" width="7" height="18" rx="1.2" fill="#e8a317" />
      <rect x="13" y="7" width="6" height="8" rx="1.2" fill="#c9c2b4" />
      <rect x="13" y="17" width="6" height="8" rx="1.2" fill="#7cb389" />
      <rect x="21" y="7" width="6" height="18" rx="1.2" fill="#d4522a" />
      <circle cx="24" cy="22" r="4" fill="#e8a317" />
      <path d="M22.4 21.7h3.2l-1.6 1.9z" fill="#12110e" />
    </svg>
  );
}
