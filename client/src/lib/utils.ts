export { cn } from "cn"

// Numeric text input: digits + one dot, max `places` decimals; "." becomes "0.". null = reject the keystroke.
export function decimalInput(v: string, places = 4): string | null {
  const s = v.replace(/,/g, ".").replace(/\s/g, "");
  if (!new RegExp(`^\\d*\\.?\\d{0,${places}}$`).test(s)) return null;
  return s.startsWith(".") ? `0${s}` : s;
}
