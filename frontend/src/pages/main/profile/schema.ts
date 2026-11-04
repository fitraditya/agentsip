import { z } from "zod";

export const emailFormSchema = z.object({
  email: z.string().trim().email("Enter a valid email address"),
  current_password: z.string().min(1, "Current password is required"),
});

export type EmailFormData = z.infer<typeof emailFormSchema>;

export const passwordFormSchema = z
  .object({
    current_password: z.string().min(1, "Current password is required"),
    new_password: z.string().min(8, "Password must be at least 8 characters"),
    confirm_password: z.string().min(1, "Please confirm the new password"),
  })
  .refine((data) => data.new_password === data.confirm_password, {
    message: "Passwords do not match",
    path: ["confirm_password"],
  });

export type PasswordFormData = z.infer<typeof passwordFormSchema>;
