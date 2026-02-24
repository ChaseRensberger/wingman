import type { FormationDefinition } from "@/lib/wingman";

export function buildDeepResearchDefinition(
  workerCount: number,
): FormationDefinition {
  return {
    name: "deep-research",
    version: 1,
    description: "Multi-agent deep research report pipeline",
    defaults: {
      work_dir: "/home/chase/Projects/wingman/resources",
    },
    nodes: [
      {
        id: "planner",
        kind: "agent",
        role: "Planner",
        agent: {
          name: "Planner",
          provider: "anthropic",
          model: "claude-haiku-4-5",
          options: {
            max_tokens: 2800,
            max_retries: 6,
          },
          instructions:
            "You are the overseer of a deep research report.\nUse perplexity_search for initial research.\nYou may call perplexity_search at most 3 times total.\nKeep tool outputs concise and summarized; never paste large raw source text.\nBuild an outline with no more than 3 non-conclusion sections.\nThe sections array MUST include those sections plus a final Conclusion section.\nConclusion requirements: id must be `conclusion`, title must be `Conclusion`, marker must be `SECTION:conclusion`.\nBefore your final response, you MUST call write exactly once to create ./report.md with non-empty markdown content.\nThe write must create a compact skeleton only (title, table of contents, and section stubs).\nEach section stub MUST be wrapped in unique marker comments exactly like:\n<!-- SECTION:{section_id}:START -->\n## {section_title}\n_TODO: {section_id}_\n<!-- SECTION:{section_id}:END -->\nAlso emit each section.marker value equal to `SECTION:{section_id}`.\nDo not write full section prose in planner.\nDo not return final JSON until the write call succeeds.\nEmit structured JSON with sections, report_path, and write_confirmed for downstream fanout.",
          tools: ["perplexity_search", "write", "edit"],
          output_schema: {
            type: "object",
            additionalProperties: false,
            required: ["sections", "report_path", "write_confirmed"],
            properties: {
              report_path: {
                type: "string",
                enum: ["./report.md"],
              },
              write_confirmed: {
                type: "boolean",
                enum: [true],
              },
              sections: {
                type: "array",
                minItems: 1,
                items: {
                  type: "object",
                  additionalProperties: false,
                  required: ["id", "title", "guidance", "marker"],
                  properties: {
                    id: { type: "string" },
                    title: { type: "string" },
                    guidance: { type: "string" },
                    marker: { type: "string" },
                  },
                },
              },
            },
          },
        },
      },
      {
        id: "iterative_research",
        kind: "fleet",
        role: "IterativeResearcher",
        fleet: {
          worker_count: workerCount,
          fanout_from: "planner.sections",
          task_mapping: {
            section_id: "item.id",
            section_title: "item.title",
            section_guidance: "item.guidance",
            section_marker: "item.marker",
          },
          agent: {
            name: "IterativeResearcher",
            provider: "anthropic",
            model: "claude-haiku-4-5",
            options: {
              max_tokens: 1400,
              max_retries: 6,
            },
            instructions:
              "You are assigned one section of ./report.md.\nDo targeted research with perplexity_search.\nYou may call perplexity_search at most 3 times for this section.\nConcisely summarize findings; do not include large quoted source text.\nIf section_id is `conclusion`, synthesize the report into a complete final conclusion (no TODO placeholders).\nEdit only your section marker block.\nYour marker namespace is provided as section_marker (example: SECTION:section_1).\nUse exactly one edit call with:\n- old_string = full current block between <!-- {section_marker}:START --> and <!-- {section_marker}:END -->\n- new_string = same markers and heading, but replace TODO body with final content\nDo not edit outside your marker block.\nReturn structured JSON when finished.",
            tools: ["perplexity_search", "read", "edit"],
            output_schema: {
              type: "object",
              additionalProperties: false,
              required: ["section_id", "status"],
              properties: {
                section_id: { type: "string" },
                status: {
                  type: "string",
                  enum: ["done"],
                },
              },
            },
          },
        },
      },
      {
        id: "proofreader",
        kind: "agent",
        role: "Proofreader",
        agent: {
          name: "Proofreader",
          provider: "anthropic",
          model: "claude-haiku-4-5",
          options: {
            max_tokens: 700,
            max_retries: 6,
          },
          instructions:
            "You are the final proofreader for ./report.md.\nPerform only a light spelling, grammar, and readability pass.\nDo not add or remove sections, do not rewrite structure, and do not do additional research.\nThe iterative researchers are responsible for completing all section content, including the conclusion.\nUse edit tool only for small targeted corrections and preserve existing meaning.\nReturn structured JSON status.",
          tools: ["read", "edit"],
          output_schema: {
            type: "object",
            additionalProperties: false,
            required: ["status"],
            properties: {
              status: {
                type: "string",
                enum: ["done"],
              },
            },
          },
        },
      },
    ],
    edges: [
      {
        from: "planner",
        to: "iterative_research",
        map: {
          sections: "output.sections",
        },
      },
      {
        from: "iterative_research",
        to: "proofreader",
        when: "all_workers_done",
        map: {
          completed_sections: "output.completed",
        },
      },
    ],
  };
}
