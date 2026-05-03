import {
  Progress,
  ProgressIndicator,
  ProgressLabel,
  ProgressTrack,
  ProgressValue,
} from "@/components/core/progress"

export function ProgressShowcase() {
  return (
    <section className="py-4 space-y-8">
      <h2 className="text-2xl font-semibold">Progress</h2>
      <div className="space-y-4">
        <div className="max-w-sm space-y-4">
          <Progress value={33}>
            <ProgressTrack>
              <ProgressIndicator />
            </ProgressTrack>
          </Progress>
          <Progress value={66}>
            <div className="flex items-center justify-between">
              <ProgressLabel>Upload progress</ProgressLabel>
              <ProgressValue />
            </div>
            <ProgressTrack>
              <ProgressIndicator />
            </ProgressTrack>
          </Progress>
          <Progress value={100}>
            <ProgressTrack>
              <ProgressIndicator />
            </ProgressTrack>
          </Progress>
        </div>
      </div>
    </section>
  )
}
