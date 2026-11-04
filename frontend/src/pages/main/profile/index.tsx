import { ComponentCard } from "@/components/shared/component-card";
import { PageBreadcrumb } from "@/components/shared/page-breadcrumb";
import { PageMeta } from "@/components/shared/page-meta";
import { Alert, AlertDescription, AlertTitle } from "@/components/ui/alert";
import { Avatar, AvatarFallback } from "@/components/ui/avatar";
import { Button } from "@/components/ui/button";
import {
  Field,
  FieldDescription,
  FieldError,
  FieldLabel,
} from "@/components/ui/field";
import { Input } from "@/components/ui/input";
import { useAuth } from "@/contexts";
import { AlertIcon } from "@/icons";
import { ApiError, authApi } from "@/lib/api";
import { zodResolver } from "@hookform/resolvers/zod";
import { useState } from "react";
import { useForm } from "react-hook-form";
import {
  emailFormSchema,
  passwordFormSchema,
  type EmailFormData,
  type PasswordFormData,
} from "./schema";

export default function ProfilePage() {
  const { user } = useAuth();
  const initial = (user?.email?.charAt(0) ?? "?").toUpperCase();

  return (
    <div>
      <PageMeta
        title="Profile | AgentSIP"
        description="Manage your account email and password."
      />
      <PageBreadcrumb pageTitle="Profile" />

      <div className="mb-6 flex items-center gap-4">
        <Avatar size="lg">
          <AvatarFallback className="bg-brand-50 text-lg font-medium text-brand-600 dark:bg-brand-500/15 dark:text-brand-400">
            {initial}
          </AvatarFallback>
        </Avatar>
        <div>
          <p className="font-medium text-gray-800 dark:text-white/90">
            {user?.email ?? "—"}
          </p>
          <p className="text-sm text-gray-500 dark:text-gray-400">
            {user?.email_verified ? "Email verified" : "Email not verified"}
          </p>
        </div>
      </div>

      <div className="grid grid-cols-1 gap-6 xl:grid-cols-2">
        <EmailCard />
        <PasswordCard />
      </div>
    </div>
  );
}

function EmailCard() {
  const { user, refreshUser } = useAuth();
  const [serverError, setServerError] = useState<string | null>(null);
  const [successMessage, setSuccessMessage] = useState<string | null>(null);
  const [submitting, setSubmitting] = useState(false);

  const {
    register,
    handleSubmit,
    reset,
    formState: { errors },
  } = useForm<EmailFormData>({
    resolver: zodResolver(emailFormSchema),
    defaultValues: { email: user?.email ?? "", current_password: "" },
  });

  const onSubmit = async (data: EmailFormData) => {
    setServerError(null);
    setSuccessMessage(null);
    setSubmitting(true);
    try {
      await authApi.updateEmail(data);
      await refreshUser();
      setSuccessMessage("Email updated.");
      reset({ email: data.email, current_password: "" });
    } catch (err) {
      setServerError(
        err instanceof ApiError ? err.message : "Something went wrong.",
      );
    } finally {
      setSubmitting(false);
    }
  };

  return (
    <ComponentCard title="Change email">
      <form onSubmit={handleSubmit(onSubmit)} className="space-y-5" noValidate>
        {serverError && (
          <Alert variant="destructive">
            <AlertIcon />
            <AlertTitle>Could not update email</AlertTitle>
            <AlertDescription>{serverError}</AlertDescription>
          </Alert>
        )}
        {successMessage && (
          <Alert variant="success">
            <AlertDescription>{successMessage}</AlertDescription>
          </Alert>
        )}

        <Field data-invalid={Boolean(errors.email)} className="gap-1.5">
          <FieldLabel>Email</FieldLabel>
          <Input
            {...register("email")}
            type="email"
            aria-invalid={errors.email ? "true" : undefined}
          />
          <FieldError errors={[errors.email]} />
        </Field>

        <Field
          data-invalid={Boolean(errors.current_password)}
          className="gap-1.5"
        >
          <FieldLabel>Current password</FieldLabel>
          <Input
            {...register("current_password")}
            type="password"
            autoComplete="current-password"
            aria-invalid={errors.current_password ? "true" : undefined}
          />
          <FieldError errors={[errors.current_password]} />
          <FieldDescription>Required to confirm this change.</FieldDescription>
        </Field>

        <div className="flex justify-end">
          <Button type="submit" disabled={submitting}>
            {submitting ? "Saving…" : "Save email"}
          </Button>
        </div>
      </form>
    </ComponentCard>
  );
}

function PasswordCard() {
  const [serverError, setServerError] = useState<string | null>(null);
  const [successMessage, setSuccessMessage] = useState<string | null>(null);
  const [submitting, setSubmitting] = useState(false);

  const {
    register,
    handleSubmit,
    reset,
    formState: { errors },
  } = useForm<PasswordFormData>({
    resolver: zodResolver(passwordFormSchema),
    defaultValues: {
      current_password: "",
      new_password: "",
      confirm_password: "",
    },
  });

  const onSubmit = async (data: PasswordFormData) => {
    setServerError(null);
    setSuccessMessage(null);
    setSubmitting(true);
    try {
      await authApi.changePassword({
        current_password: data.current_password,
        new_password: data.new_password,
      });
      setSuccessMessage("Password updated.");
      reset();
    } catch (err) {
      setServerError(
        err instanceof ApiError ? err.message : "Something went wrong.",
      );
    } finally {
      setSubmitting(false);
    }
  };

  return (
    <ComponentCard title="Change password">
      <form onSubmit={handleSubmit(onSubmit)} className="space-y-5" noValidate>
        {serverError && (
          <Alert variant="destructive">
            <AlertIcon />
            <AlertTitle>Could not update password</AlertTitle>
            <AlertDescription>{serverError}</AlertDescription>
          </Alert>
        )}
        {successMessage && (
          <Alert variant="success">
            <AlertDescription>{successMessage}</AlertDescription>
          </Alert>
        )}

        <Field
          data-invalid={Boolean(errors.current_password)}
          className="gap-1.5"
        >
          <FieldLabel>Current password</FieldLabel>
          <Input
            {...register("current_password")}
            type="password"
            autoComplete="current-password"
            aria-invalid={errors.current_password ? "true" : undefined}
          />
          <FieldError errors={[errors.current_password]} />
        </Field>

        <Field data-invalid={Boolean(errors.new_password)} className="gap-1.5">
          <FieldLabel>New password</FieldLabel>
          <Input
            {...register("new_password")}
            type="password"
            autoComplete="new-password"
            aria-invalid={errors.new_password ? "true" : undefined}
          />
          <FieldError errors={[errors.new_password]} />
        </Field>

        <Field
          data-invalid={Boolean(errors.confirm_password)}
          className="gap-1.5"
        >
          <FieldLabel>Confirm new password</FieldLabel>
          <Input
            {...register("confirm_password")}
            type="password"
            autoComplete="new-password"
            aria-invalid={errors.confirm_password ? "true" : undefined}
          />
          <FieldError errors={[errors.confirm_password]} />
        </Field>

        <div className="flex justify-end">
          <Button type="submit" disabled={submitting}>
            {submitting ? "Saving…" : "Save password"}
          </Button>
        </div>
      </form>
    </ComponentCard>
  );
}
