import { useState } from "react";
import { ChevronsUpDown } from "lucide-react";
import { Button } from "@/components/ui/button";
import {
  Dialog,
  DialogContent,
  DialogHeader,
  DialogTitle,
} from "@/components/ui/dialog";
import { NamespacePicker } from "@/components/domain/namespace-picker";
import { useNamespace } from "@/lib/namespace-context";

export function NamespaceSwitcher() {
  const { namespace } = useNamespace();
  const [open, setOpen] = useState(false);

  if (!namespace) return null;

  return (
    <>
      <Button
        variant="outline"
        size="sm"
        className="h-8 gap-1.5 text-sm"
        onClick={() => setOpen(true)}
      >
        {namespace}
        <ChevronsUpDown className="h-3 w-3 text-muted-foreground" />
      </Button>
      <Dialog open={open} onOpenChange={setOpen}>
        <DialogContent className="sm:max-w-lg">
          <DialogHeader>
            <DialogTitle>切换命名空间</DialogTitle>
          </DialogHeader>
          <NamespacePicker variant="table" onSelected={() => setOpen(false)} />
        </DialogContent>
      </Dialog>
    </>
  );
}
