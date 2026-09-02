// PollError renders the state a page shows when its primary poll fails:
// previously pages kept their skeleton forever while the poller refetched a
// dead gateway (finding M15).
export default function PollError({
  message,
  onRetry,
}: {
  message: string
  onRetry: () => void
}) {
  return (
    <div className="p-8 flex justify-center">
      <div className="bg-error/8 border border-error/20 rounded-xl p-8 max-w-lg w-full text-center">
        <div className="text-[15px] font-semibold text-ink mb-1">
          Cannot reach the gateway
        </div>
        <p className="text-[13px] text-ink-muted font-mono break-words mb-4">
          {message}
        </p>
        <button
          onClick={onRetry}
          className="px-3 py-1.5 rounded-lg bg-surface-2 hover:bg-rs-accent hover:text-white text-[13px] font-medium transition-colors"
        >
          Retry
        </button>
      </div>
    </div>
  )
}
