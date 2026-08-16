declare module '@nimiq/identicons' {
  const Identicons: {
    svg(text: string): Promise<string>
    toDataUrl(text: string): Promise<string>
    render(text: string, element: HTMLElement): Promise<void>
    image(text: string): Promise<HTMLImageElement>
    placeholder(color?: string, strokeWidth?: number): string
    placeholderToDataUrl(color?: string, strokeWidth?: number): string
    renderPlaceholder(element: HTMLElement, color?: string, strokeWidth?: number): void
  }
  export default Identicons
}
