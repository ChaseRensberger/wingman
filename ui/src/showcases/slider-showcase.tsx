import {
  Slider,
  SliderControl,
  SliderIndicator,
  SliderLabel,
  SliderThumb,
  SliderTrack,
  SliderValue,
} from '@/components/core/slider'

export function SliderShowcase() {
  return (
    <section className="py-4 space-y-8">
      <h2 className="text-2xl font-semibold">Slider</h2>
      <div className="space-y-6 max-w-sm">
        <Slider defaultValue={[33]} max={100} step={1}>
          <div className="flex items-center justify-between">
            <SliderLabel>Volume</SliderLabel>
            <SliderValue />
          </div>
          <SliderControl>
            <SliderTrack>
              <SliderIndicator />
              <SliderThumb />
            </SliderTrack>
          </SliderControl>
        </Slider>
        <Slider defaultValue={[20, 80]} max={100} step={1}>
          <div className="flex items-center justify-between">
            <SliderLabel>Price Range</SliderLabel>
            <SliderValue />
          </div>
          <SliderControl>
            <SliderTrack>
              <SliderIndicator />
              <SliderThumb />
              <SliderThumb />
            </SliderTrack>
          </SliderControl>
        </Slider>
        <Slider defaultValue={[50]} max={100} step={1} disabled>
          <div className="flex items-center justify-between">
            <SliderLabel>Disabled</SliderLabel>
            <SliderValue />
          </div>
          <SliderControl>
            <SliderTrack>
              <SliderIndicator />
              <SliderThumb />
            </SliderTrack>
          </SliderControl>
        </Slider>
      </div>
    </section>
  )
}
