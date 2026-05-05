import {
  TypographyH1,
  TypographyH2,
  TypographyH3,
  TypographyH4,
  TypographyP,
  TypographyLead,
  TypographyLarge,
  TypographySmall,
  TypographyMuted,
  TypographyCode,
} from "@/components/core/typography"

export function TypographyShowcase() {
  return (
    <section className="py-4 space-y-8">
      <h2 className="text-2xl font-semibold">Typography</h2>
      <div className="space-y-4 max-w-lg">
        <TypographyH1>Heading 1</TypographyH1>
        <TypographyH2>Heading 2</TypographyH2>
        <TypographyH3>Heading 3</TypographyH3>
        <TypographyH4>Heading 4</TypographyH4>
        <TypographyLead>A lead paragraph for introducing sections.</TypographyLead>
        <TypographyP>Regular paragraph text for body content. The quick brown fox jumps over the lazy dog.</TypographyP>
        <TypographyLarge>Large text for emphasis</TypographyLarge>
        <TypographySmall>Small text for captions</TypographySmall>
        <TypographyMuted>Muted text for secondary info</TypographyMuted>
        <TypographyCode>inline code snippet</TypographyCode>
      </div>
    </section>
  )
}
