export function shortPath(p: string): string {
  return p.replace(/^[A-Za-z]:\\Users\\[^\\]+/, '~').replace(/^\/Users\/[^/]+/, '~')
}

export function bytes(n: number): string {
  if (!n) return '0 B'
  const units = ['B', 'KB', 'MB', 'GB']
  let i = 0
  let v = n
  while (v >= 1024 && i < units.length - 1) {
    v /= 1024
    i++
  }
  return `${v.toFixed(i === 0 ? 0 : 1)} ${units[i]}`
}

export function slug(s: string): string {
  return s
    .toLowerCase()
    .trim()
    .replace(/[^a-z0-9]+/g, '-')
    .replace(/^-+|-+$/g, '')
}

/** phpMyAdmin URL honouring a non-default Apache HTTP port. */
export function phpMyAdminURL(httpPort?: number): string {
  return `http://localhost${httpPort && httpPort !== 80 ? `:${httpPort}` : ''}/phpmyadmin`
}

export function frameworkLabel(fw: string): string {
  switch (fw) {
    case 'laravel':
      return 'Laravel'
    case 'wordpress':
      return 'WordPress'
    default:
      return 'Plain PHP'
  }
}

export function basename(p: string): string {
  const parts = p.split(/[\\/]/).filter(Boolean)
  return parts[parts.length - 1] ?? p
}

export function joinPath(dir: string, name: string): string {
  const sep = dir.includes('/') && !dir.includes('\\') ? '/' : '\\'
  return dir.replace(/[\\/]+$/, '') + sep + name
}

export async function copyText(text: string): Promise<void> {
  try {
    await navigator.clipboard.writeText(text)
  } catch {
    const ta = document.createElement('textarea')
    ta.value = text
    document.body.appendChild(ta)
    ta.select()
    document.execCommand('copy')
    ta.remove()
  }
}
