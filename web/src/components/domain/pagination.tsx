import { Button } from "@/components/ui/button";

type PaginationProps = {
  page: number;
  pageSize: number;
  totalCount: number;
  onPageChange: (page: number) => void;
};

export function Pagination({
  page,
  pageSize,
  totalCount,
  onPageChange,
}: PaginationProps) {
  const totalPages = Math.max(
    1,
    Math.ceil(totalCount / Math.max(pageSize, 1)),
  );

  return (
    <div className="flex items-center justify-between pt-4 text-sm text-muted-foreground">
      <span>
        {totalCount} 条 · 第 {page} / {totalPages} 页
      </span>
      <div className="flex gap-2">
        <Button
          variant="outline"
          size="sm"
          disabled={page <= 1}
          onClick={() => onPageChange(page - 1)}
        >
          上一页
        </Button>
        <Button
          variant="outline"
          size="sm"
          disabled={page >= totalPages}
          onClick={() => onPageChange(page + 1)}
        >
          下一页
        </Button>
      </div>
    </div>
  );
}
