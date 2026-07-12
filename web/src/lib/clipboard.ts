import { toast } from "sonner";

export async function copyToClipboard(text: string, message = "已复制") {
  try {
    await navigator.clipboard.writeText(text);
    toast.success(message);
  } catch {
    toast.error("复制失败");
  }
}
