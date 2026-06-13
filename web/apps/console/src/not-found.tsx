import { Link } from "@tanstack/react-router";
import { Warning } from "@phosphor-icons/react";
import { Button } from "@wingman/core/components/core/button";
import { Empty, EmptyActions, EmptyDescription, EmptyIcon, EmptyTitle } from "@wingman/core/components/core/empty";

export default function NotFoundPage() {
  return (
    <div className="flex flex-1 items-center justify-center">
      <Empty>
        <EmptyIcon>
          <Warning weight="bold" />
        </EmptyIcon>
        <EmptyTitle>404 — Page not found</EmptyTitle>
        <EmptyDescription>
          The page you are looking for does not exist or has been moved.
        </EmptyDescription>
        <EmptyActions>
          <Button render={<Link to="/" />} nativeButton={false}>Go home</Button>
        </EmptyActions>
      </Empty>
    </div>
  );
}
