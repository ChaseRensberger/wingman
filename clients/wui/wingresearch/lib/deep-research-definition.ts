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
            max_tokens: 1200,
          },
          instructions:
            "You are the overseer of a deep research report.\nUse perplexity_search for initial research.\nYou may call perplexity_search at most 3 times total.\nKeep tool outputs concise and summarized; never paste large raw source text.\nBuild an outline with no more than 3 sections (excluding Conclusion).\nCreate ./report.md with a table of contents and section stubs.\nEmit structured JSON with a sections array for downstream fanout.",
          tools: ["perplexity_search", "write", "edit"],
          output_schema: {
            type: "object",
            additionalProperties: false,
            required: ["sections"],
            properties: {
              sections: {
                type: "array",
                minItems: 1,
                items: {
                  type: "object",
                  additionalProperties: false,
                  required: ["id", "title", "guidance"],
                  properties: {
                    id: { type: "string" },
                    title: { type: "string" },
                    guidance: { type: "string" },
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
          },
          agent: {
            name: "IterativeResearcher",
            provider: "anthropic",
            model: "claude-haiku-4-5",
            options: {
              max_tokens: 900,
            },
            instructions:
              "You are assigned one section of ./report.md.\nDo targeted research with perplexity_search.\nYou may call perplexity_search at most 3 times for this section.\nConcisely summarize findings; do not include large quoted source text.\nFill only your assigned section.\nReturn structured JSON when finished.",
            tools: ["perplexity_search", "edit"],
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
          },
          instructions:
            "Do a final proofreading and quality pass over ./report.md.\nImprove spelling, structure, and readability without changing intent.\nReturn structured JSON status.",
          tools: ["edit"],
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
