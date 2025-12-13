import { useState, useEffect } from "react";
import { zodResolver } from "@hookform/resolvers/zod";
import { useForm } from "react-hook-form";
import { z } from "zod";
import { Button } from "@/components/ui/button";
import {
  Table,
  TableBody,
  TableCell,
  TableHead,
  TableHeader,
  TableRow
} from "@/components/ui/table";
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogFooter,
  DialogHeader,
  DialogTitle,
} from "@/components/ui/dialog";
import { Input } from "@/components/ui/input";
import {
  Form,
  FormControl,
  FormField,
  FormItem,
  FormLabel,
  FormMessage,
} from "@/components/ui/form";
import { Select, SelectContent, SelectItem, SelectTrigger, SelectValue } from "@/components/ui/select";
import {
  AlertDialog,
  AlertDialogAction,
  AlertDialogCancel,
  AlertDialogContent,
  AlertDialogDescription,
  AlertDialogFooter,
  AlertDialogHeader,
  AlertDialogTitle,
  AlertDialogTrigger,
} from "@/components/ui/alert-dialog";
import { Badge } from "@/components/ui/badge";
import { Card, CardContent, CardDescription, CardHeader, CardTitle } from "@/components/ui/card";
import { Plus, Edit, Trash2, Layers } from "lucide-react";
import { toast } from "sonner";

const modelMappingSchema = z.object({
  channel_id: z.number().min(1, "请选择Channel"),
  virtual_model: z.string().min(1, "虚拟模型名称不能为空"),
  actual_model: z.string().min(1, "实际模型名称不能为空"),
  protocol: z.string().min(1, "请选择协议"),
  weight: z.number().min(1, "权重必须大于0").default(1),
  status: z.string().default("active"),
});

type ModelMappingFormData = z.infer<typeof modelMappingSchema>;

interface ModelMapping {
  id: number;
  channel_id: number;
  channel: {
    id: number;
    name: string;
    provider: {
      name: string;
    };
  };
  virtual_model: string;
  actual_model: string;
  protocol: string;
  weight: number;
  status: string;
  request_count: number;
  success_count: number;
  error_count: number;
  avg_response_time: number;
  last_used_at?: string;
}

interface Channel {
  id: number;
  name: string;
  provider_name: string;
  supported_protocols: string[];
}

interface Protocol {
  type: string;
  name: string;
  description: string;
}

export default function ModelMappings() {
  const [mappings, setMappings] = useState<ModelMapping[]>([]);
  const [channels, setChannels] = useState<Channel[]>([]);
  const [protocols, setProtocols] = useState<Protocol[]>([]);
  const [isDialogOpen, setIsDialogOpen] = useState(false);
  const [editingMapping, setEditingMapping] = useState<ModelMapping | null>(null);
  const [loading, setLoading] = useState(true);
  const [selectedChannelId, setSelectedChannelId] = useState<number | null>(null);

  const form = useForm<ModelMappingFormData>({
    resolver: zodResolver(modelMappingSchema),
    defaultValues: {
      channel_id: 0,
      virtual_model: "",
      actual_model: "",
      protocol: "openai",
      weight: 1,
      status: "active",
    },
  });

  const fetchMappings = async () => {
    try {
      const url = selectedChannelId
        ? `/api/model-mappings?channel_id=${selectedChannelId}`
        : "/api/model-mappings";
      const response = await fetch(url);
      if (response.ok) {
        const data = await response.json();
        setMappings(data.data || []);
      }
    } catch (error) {
      toast.error("获取模型映射列表失败");
    }
  };

  const fetchChannels = async () => {
    try {
      const response = await fetch("/api/channels");
      if (response.ok) {
        const data = await response.json();
        setChannels(data.data || []);
      }
    } catch (error) {
      toast.error("获取Channel列表失败");
    }
  };

  const fetchProtocols = async () => {
    try {
      const response = await fetch("/api/supported-protocols");
      if (response.ok) {
        const data = await response.json();
        setProtocols(data.data || []);
      }
    } catch (error) {
      toast.error("获取协议列表失败");
    }
  };

  useEffect(() => {
    const loadData = async () => {
      setLoading(true);
      await Promise.all([
        fetchMappings(),
        fetchChannels(),
        fetchProtocols(),
      ]);
      setLoading(false);
    };
    loadData();
  }, [selectedChannelId]);

  const handleSubmit = async (data: ModelMappingFormData) => {
    try {
      const url = editingMapping ? `/api/model-mappings/${editingMapping.id}` : "/api/model-mappings";
      const method = editingMapping ? "PUT" : "POST";

      const response = await fetch(url, {
        method,
        headers: { "Content-Type": "application/json" },
        body: JSON.stringify(data),
      });

      if (response.ok) {
        toast.success(editingMapping ? "模型映射更新成功" : "模型映射创建成功");
        setIsDialogOpen(false);
        setEditingMapping(null);
        form.reset();
        fetchMappings();
      } else {
        const error = await response.json();
        toast.error(error.message || "操作失败");
      }
    } catch (error) {
      toast.error("网络错误");
    }
  };

  const handleEdit = (mapping: ModelMapping) => {
    setEditingMapping(mapping);
    form.reset({
      channel_id: mapping.channel_id,
      virtual_model: mapping.virtual_model,
      actual_model: mapping.actual_model,
      protocol: mapping.protocol,
      weight: mapping.weight,
      status: mapping.status,
    });
    setIsDialogOpen(true);
  };

  const handleDelete = async (id: number) => {
    try {
      const response = await fetch(`/api/model-mappings/${id}`, { method: "DELETE" });
      if (response.ok) {
        toast.success("模型映射删除成功");
        fetchMappings();
      } else {
        const error = await response.json();
        toast.error(error.message || "删除失败");
      }
    } catch (error) {
      toast.error("网络错误");
    }
  };

  const getStatusBadge = (status: string) => {
    switch (status) {
      case "active":
        return <Badge variant="default">活跃</Badge>;
      case "inactive":
        return <Badge variant="secondary">停用</Badge>;
      default:
        return <Badge variant="outline">{status}</Badge>;
    }
  };

  const getErrorRate = (mapping: ModelMapping) => {
    if (mapping.request_count === 0) return 0;
    return ((mapping.error_count / mapping.request_count) * 100);
  };

  if (loading) {
    return <div className="flex justify-center items-center h-64">加载中...</div>;
  }

  return (
    <div className="space-y-6">
      <div className="flex justify-between items-center">
        <div>
          <h1 className="text-3xl font-bold">模型映射管理</h1>
          <p className="text-muted-foreground">管理虚拟模型到实际模型的映射关系</p>
        </div>
        <Button onClick={() => setIsDialogOpen(true)}>
          <Plus className="mr-2 h-4 w-4" />
          创建映射
        </Button>
      </div>

      <div className="grid grid-cols-1 md:grid-cols-4 gap-4">
        <Card>
          <CardHeader className="flex flex-row items-center justify-between space-y-0 pb-2">
            <CardTitle className="text-sm font-medium">总映射数</CardTitle>
            <Layers className="h-4 w-4 text-muted-foreground" />
          </CardHeader>
          <CardContent>
            <div className="text-2xl font-bold">{mappings.length}</div>
          </CardContent>
        </Card>
        <Card>
          <CardHeader className="flex flex-row items-center justify-between space-y-0 pb-2">
            <CardTitle className="text-sm font-medium">活跃映射</CardTitle>
            <Layers className="h-4 w-4 text-green-600" />
          </CardHeader>
          <CardContent>
            <div className="text-2xl font-bold">
              {mappings.filter(m => m.status === "active").length}
            </div>
          </CardContent>
        </Card>
        <Card>
          <CardHeader className="flex flex-row items-center justify-between space-y-0 pb-2">
            <CardTitle className="text-sm font-medium">总请求数</CardTitle>
            <Layers className="h-4 w-4 text-blue-600" />
          </CardHeader>
          <CardContent>
            <div className="text-2xl font-bold">
              {mappings.reduce((sum, m) => sum + m.request_count, 0)}
            </div>
          </CardContent>
        </Card>
        <Card>
          <CardHeader className="flex flex-row items-center justify-between space-y-0 pb-2">
            <CardTitle className="text-sm font-medium">平均成功率</CardTitle>
            <Layers className="h-4 w-4 text-green-600" />
          </CardHeader>
          <CardContent>
            <div className="text-2xl font-bold">
              {mappings.length > 0
                ? (mappings.reduce((sum, m) => {
                    const successRate = m.request_count > 0 ? (m.success_count / m.request_count) : 0;
                    return sum + successRate;
                  }, 0) / mappings.length * 100).toFixed(1)
                : 0}%
            </div>
          </CardContent>
        </Card>
      </div>

      <Card>
        <CardHeader>
          <CardTitle>模型映射列表</CardTitle>
          <CardDescription>
            <div className="flex items-center gap-4">
              <span>管理所有模型映射的配置和统计</span>
              <Select onValueChange={(value) => setSelectedChannelId(value === "all" ? null : parseInt(value))}>
                <SelectTrigger className="w-48">
                  <SelectValue placeholder="筛选Channel" />
                </SelectTrigger>
                <SelectContent>
                  <SelectItem value="all">所有Channel</SelectItem>
                  {channels.map((channel) => (
                    <SelectItem key={channel.id} value={channel.id.toString()}>
                      {channel.name}
                    </SelectItem>
                  ))}
                </SelectContent>
              </Select>
            </div>
          </CardDescription>
        </CardHeader>
        <CardContent>
          <Table>
            <TableHeader>
              <TableRow>
                <TableHead>虚拟模型</TableHead>
                <TableHead>实际模型</TableHead>
                <TableHead>Channel</TableHead>
                <TableHead>协议</TableHead>
                <TableHead>权重</TableHead>
                <TableHead>状态</TableHead>
                <TableHead>请求数</TableHead>
                <TableHead>错误率</TableHead>
                <TableHead>平均响应时间</TableHead>
                <TableHead>操作</TableHead>
              </TableRow>
            </TableHeader>
            <TableBody>
              {mappings.map((mapping) => (
                <TableRow key={mapping.id}>
                  <TableCell className="font-medium">{mapping.virtual_model}</TableCell>
                  <TableCell>{mapping.actual_model}</TableCell>
                  <TableCell>
                    <div>
                      <div className="font-medium">{mapping.channel.name}</div>
                      <div className="text-sm text-muted-foreground">
                        {mapping.channel.provider.name}
                      </div>
                    </div>
                  </TableCell>
                  <TableCell>
                    <Badge variant="outline">{mapping.protocol}</Badge>
                  </TableCell>
                  <TableCell>{mapping.weight}</TableCell>
                  <TableCell>{getStatusBadge(mapping.status)}</TableCell>
                  <TableCell>{mapping.request_count}</TableCell>
                  <TableCell>
                    <span className={getErrorRate(mapping) < 5 ? "text-green-600" : getErrorRate(mapping) < 15 ? "text-yellow-600" : "text-red-600"}>
                      {getErrorRate(mapping).toFixed(1)}%
                    </span>
                  </TableCell>
                  <TableCell>{mapping.avg_response_time}ms</TableCell>
                  <TableCell>
                    <div className="flex gap-2">
                      <Button
                        variant="outline"
                        size="sm"
                        onClick={() => handleEdit(mapping)}
                      >
                        <Edit className="h-4 w-4" />
                      </Button>
                      <AlertDialog>
                        <AlertDialogTrigger asChild>
                          <Button variant="outline" size="sm">
                            <Trash2 className="h-4 w-4" />
                          </Button>
                        </AlertDialogTrigger>
                        <AlertDialogContent>
                          <AlertDialogHeader>
                            <AlertDialogTitle>确认删除</AlertDialogTitle>
                            <AlertDialogDescription>
                              确定要删除模型映射 "{mapping.virtual_model} → {mapping.actual_model}" 吗？此操作不可撤销。
                            </AlertDialogDescription>
                          </AlertDialogHeader>
                          <AlertDialogFooter>
                            <AlertDialogCancel>取消</AlertDialogCancel>
                            <AlertDialogAction onClick={() => handleDelete(mapping.id)}>
                              删除
                            </AlertDialogAction>
                          </AlertDialogFooter>
                        </AlertDialogContent>
                      </AlertDialog>
                    </div>
                  </TableCell>
                </TableRow>
              ))}
            </TableBody>
          </Table>
        </CardContent>
      </Card>

      <Dialog open={isDialogOpen} onOpenChange={setIsDialogOpen}>
        <DialogContent className="max-w-md">
          <DialogHeader>
            <DialogTitle>{editingMapping ? "编辑模型映射" : "创建模型映射"}</DialogTitle>
            <DialogDescription>
              配置虚拟模型到实际模型的映射关系
            </DialogDescription>
          </DialogHeader>
          <Form {...form}>
            <form onSubmit={form.handleSubmit(handleSubmit)} className="space-y-4">
              <FormField
                control={form.control}
                name="channel_id"
                render={({ field }) => (
                  <FormItem>
                    <FormLabel>Channel</FormLabel>
                    <Select onValueChange={(value) => field.onChange(parseInt(value))} value={field.value?.toString()}>
                      <FormControl>
                        <SelectTrigger>
                          <SelectValue placeholder="选择Channel" />
                        </SelectTrigger>
                      </FormControl>
                      <SelectContent>
                        {channels.map((channel) => (
                          <SelectItem key={channel.id} value={channel.id.toString()}>
                            {channel.name} ({channel.provider_name})
                          </SelectItem>
                        ))}
                      </SelectContent>
                    </Select>
                    <FormMessage />
                  </FormItem>
                )}
              />
              <FormField
                control={form.control}
                name="virtual_model"
                render={({ field }) => (
                  <FormItem>
                    <FormLabel>虚拟模型</FormLabel>
                    <FormControl>
                      <Input placeholder="如: gpt-4" {...field} />
                    </FormControl>
                    <FormMessage />
                  </FormItem>
                )}
              />
              <FormField
                control={form.control}
                name="actual_model"
                render={({ field }) => (
                  <FormItem>
                    <FormLabel>实际模型</FormLabel>
                    <FormControl>
                      <Input placeholder="如: claude-3-5-sonnet-20241022" {...field} />
                    </FormControl>
                    <FormMessage />
                  </FormItem>
                )}
              />
              <FormField
                control={form.control}
                name="protocol"
                render={({ field }) => (
                  <FormItem>
                    <FormLabel>协议</FormLabel>
                    <Select onValueChange={field.onChange} value={field.value}>
                      <FormControl>
                        <SelectTrigger>
                          <SelectValue placeholder="选择协议" />
                        </SelectTrigger>
                      </FormControl>
                      <SelectContent>
                        {protocols.map((protocol) => (
                          <SelectItem key={protocol.type} value={protocol.type}>
                            {protocol.name} - {protocol.description}
                          </SelectItem>
                        ))}
                      </SelectContent>
                    </Select>
                    <FormMessage />
                  </FormItem>
                )}
              />
              <FormField
                control={form.control}
                name="weight"
                render={({ field }) => (
                  <FormItem>
                    <FormLabel>权重</FormLabel>
                    <FormControl>
                      <Input
                        type="number"
                        min="1"
                        {...field}
                        onChange={(e) => field.onChange(parseInt(e.target.value))}
                      />
                    </FormControl>
                    <FormMessage />
                  </FormItem>
                )}
              />
              <DialogFooter>
                <Button type="button" variant="outline" onClick={() => setIsDialogOpen(false)}>
                  取消
                </Button>
                <Button type="submit">
                  {editingMapping ? "更新" : "创建"}
                </Button>
              </DialogFooter>
            </form>
          </Form>
        </DialogContent>
      </Dialog>
    </div>
  );
}