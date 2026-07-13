import { useEffect, useState } from "react";

import {
  Pagination,
  PaginationContent,
  PaginationEllipsis,
  PaginationItem,
  PaginationLink,
  PaginationNext,
  PaginationPrevious,
} from "@wingman/core/components/core/pagination";
import { cn } from "@/lib/utils";

type ClientPaginationState<T> = {
  items: T[];
  page: number;
  pageItems: T[];
  pageSize: number;
  totalItems: number;
  totalPages: number;
  startItem: number;
  endItem: number;
  setPage: (page: number) => void;
};

type ClientPaginationProps = {
  page: number;
  pageSize: number;
  totalItems: number;
  totalPages: number;
  startItem: number;
  endItem: number;
  onPageChange: (page: number) => void;
  className?: string;
};

export function useClientPagination<T>(items: T[], pageSize = 25, resetKey = ""): ClientPaginationState<T> {
  const [page, setPageState] = useState(1);
  const totalItems = items.length;
  const totalPages = Math.max(1, Math.ceil(totalItems / pageSize));
  const safePage = Math.min(page, totalPages);
  const startIndex = (safePage - 1) * pageSize;
  const endIndex = Math.min(startIndex + pageSize, totalItems);

  useEffect(() => {
    setPageState(1);
  }, [resetKey]);

  useEffect(() => {
    setPageState((current) => Math.min(current, totalPages));
  }, [totalPages]);

  return {
    items,
    page: safePage,
    pageItems: items.slice(startIndex, endIndex),
    pageSize,
    totalItems,
    totalPages,
    startItem: totalItems === 0 ? 0 : startIndex + 1,
    endItem: endIndex,
    setPage: (nextPage) => setPageState(Math.min(Math.max(1, nextPage), totalPages)),
  };
}

export function ClientPagination({
  page,
  pageSize,
  totalItems,
  totalPages,
  onPageChange,
  className,
}: ClientPaginationProps) {
  if (totalItems <= pageSize) return null;

  const pages = paginationItems(page, totalPages);

  return (
    <div className={cn("flex flex-col items-center justify-center gap-3 pt-4 text-center text-sm text-muted-foreground", className)}>
      <Pagination className="mx-0 w-auto justify-center">
        <PaginationContent>
          <PaginationItem>
            <PaginationPrevious
              href="#"
              aria-disabled={page === 1}
              className={cn(page === 1 && "pointer-events-none opacity-50")}
              onClick={(event) => {
                event.preventDefault();
                if (page > 1) onPageChange(page - 1);
              }}
            />
          </PaginationItem>
          {pages.map((item, index) => (
            <PaginationItem key={`${item}-${index}`}>
              {item === "ellipsis" ? (
                <PaginationEllipsis />
              ) : (
                <PaginationLink
                  href="#"
                  isActive={item === page}
                  onClick={(event) => {
                    event.preventDefault();
                    onPageChange(item);
                  }}
                >
                  {item}
                </PaginationLink>
              )}
            </PaginationItem>
          ))}
          <PaginationItem>
            <PaginationNext
              href="#"
              aria-disabled={page === totalPages}
              className={cn(page === totalPages && "pointer-events-none opacity-50")}
              onClick={(event) => {
                event.preventDefault();
                if (page < totalPages) onPageChange(page + 1);
              }}
            />
          </PaginationItem>
        </PaginationContent>
      </Pagination>
    </div>
  );
}

function paginationItems(page: number, totalPages: number): Array<number | "ellipsis"> {
  const pages = new Set([1, totalPages, page - 1, page, page + 1]);
  const visible = [...pages]
    .filter((item) => item >= 1 && item <= totalPages)
    .sort((a, b) => a - b);
  const items: Array<number | "ellipsis"> = [];

  for (const item of visible) {
    const previous = items[items.length - 1];
    if (typeof previous === "number" && item - previous > 1) {
      items.push("ellipsis");
    }
    items.push(item);
  }

  return items;
}
