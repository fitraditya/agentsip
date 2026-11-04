import { z } from "zod";

export const ingestDocumentSchema = z.object({
  sourceUri: z.string().trim().min(1, "Source label is required"),
  text: z.string().trim().min(1, "Document text is required"),
});

export type IngestDocumentFormData = z.infer<typeof ingestDocumentSchema>;
