import { Component, type ErrorInfo, type ReactNode } from "react"

interface Props {
  children: ReactNode
}

interface State {
  error: Error | null
}

// ErrorBoundary catches render crashes so one bad page cannot kill the whole
// SPA while the poller keeps refetching (finding M14).
export default class ErrorBoundary extends Component<Props, State> {
  state: State = { error: null }

  static getDerivedStateFromError(error: Error): State {
    return { error }
  }

  componentDidCatch(error: Error, info: ErrorInfo): void {
    console.error("Render error:", error, info.componentStack)
  }

  render() {
    if (this.state.error) {
      return (
        <div className="p-8 flex justify-center">
          <div className="bg-error/8 border border-error/20 rounded-xl p-8 max-w-lg w-full text-center">
            <div className="text-[16px] font-semibold text-ink mb-2">
              Something went wrong
            </div>
            <p className="text-[13px] text-ink-muted font-mono break-words mb-4">
              {this.state.error.message}
            </p>
            <button
              onClick={() => {
                this.setState({ error: null })
              }}
              className="px-3 py-1.5 rounded-lg bg-surface-2 hover:bg-rs-accent hover:text-white text-[13px] font-medium transition-colors"
            >
              Try again
            </button>
          </div>
        </div>
      )
    }
    return this.props.children
  }
}
