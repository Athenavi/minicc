declare module 'markdown-it-texmath' {
  import type MarkdownIt from 'markdown-it'
  interface TexMathOptions {
    engine?: any
    delimiters?: string | string[]
    katexOptions?: Record<string, any>
  }
  const texmath: (md: MarkdownIt, options?: TexMathOptions) => void
  export default texmath
}
