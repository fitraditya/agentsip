import { z } from "zod";

export const createKnowledgeBaseSchema = z
  .object({
    name: z.string().trim().min(1, "Name is required"),
    embeddingProvider: z.literal("openai_compatible"),
    embeddingBaseUrl: z
      .string()
      .trim()
      .min(1, "Base URL is required")
      .url("Enter a valid URL"),
    embeddingApiKey: z.string().trim().min(1, "API key is required"),
    embeddingModel: z.string().trim().min(1, "Model is required"),
    useCustomVectorStore: z.boolean(),
    vectorStoreDsn: z.string().trim().optional(),
  })
  .refine(
    (data) => !data.useCustomVectorStore || !!data.vectorStoreDsn?.length,
    {
      message: "A connection string is required for a custom vector store",
      path: ["vectorStoreDsn"],
    },
  );

export type CreateKnowledgeBaseFormData = z.infer<
  typeof createKnowledgeBaseSchema
>;
