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
import { Textarea } from "@/components/ui/textarea";
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
import { Plus, Edit, Trash2, RotateCcw, Activity } from "lucide-react";
import { toast } from "sonner";

const channelSchema = z.object({
  name: z.string().min(1, "渠道名称不能为空"),
  description: z.string().optional(),
  provider_id: z.number().min(1, "请选择Provider"),
  supported_protocols: z.array(z.string()).min(1, "至少选择一个协议"),
  load_balance_strategy: z.string().min(1, "请选择负载均衡策略"),
  weight: z.number().min(1, "权重必须大于0").default(1),
  status: z.string().default("active"),
});

type ChannelFormData = z.infer<typeof channelSchema>;

interface Channel {
  id: number;
  name: string;
  description: string;
  provider_id: number;
  provider_name: string;
  supported_protocols: string[];
  load_balance_strategy: string;
  weight: number;
  status: string;
  total_requests: number;
  success_requests: number;
  error_requests: number;
  error_rate: number;
  success_rate: number;
  avg_response_time: number;
  last_used_at?: string;
  cooldown_until?: string;
}

interface Provider {
  id: number;
  name: string;
  type: string;
}

interface BalancerType {
  type: string;
  name: string;
  description: string;
  advanced: boolean;
}

interface Protocol {
  type: string;
  name: string;
  description: string;
}

export default function Channels() {
  const [channels, setChannels] = useState<Channel[]>([]);
  const [providers, setProviders] = useState<Provider[]>([]);
  const [balancerTypes, setBalancerTypes] = useState<BalancerType[]>([]);
  const [protocols, setProtocols] = useState<Protocol[]>([]);
  const [isDialogOpen, setIsDialogOpen] = useState(false);
  const [editingChannel, setEditingChannel] = useState<Channel | null>(null);
  const [loading, setLoading] = useState(true);

  const form = useForm<ChannelFormData>({
    resolver: zodResolver(channelSchema),
    defaultValues: {
      name: "",
      description: "",
      provider_id: 0,
      supported_protocols: ["openai"],
      load_balance_strategy: "error_aware",
      weight: 1,
      status: "active",
    },
  });

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

  const fetchProviders = async () => {
    try {
      const response = await fetch("/api/providers");
      if (response.ok) {
        const data = await response.json();
        setProviders(data.data || []);
      }
    } catch (error) {
      toast.error("获取Provider列表失败");
    }
  };

  const fetchBalancerTypes = async () => {
    try {
      const response = await fetch("/api/balancer-types");
      if (response.ok) {
        const data = await response.json();
        setBalancerTypes(data.data || []);
      }
    } catch (error) {
      toast.error("获取负载均衡策略失败");
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
        fetchChannels(),
        fetchProviders(),
        fetchBalancerTypes(),
        fetchProtocols(),
      ]);
      setLoading(false);
    };
    loadData();
  }, []);

  const handleSubmit = async (data: ChannelFormData) => {
    try {
      const url = editingChannel ? `/api/channels/${editingChannel.id}` : "/api/channels";
      const method = editingChannel ? "PUT" : "POST";

      const response = await fetch(url, {
        method,
        headers: { "Content-Type": "application/json" },
        body: JSON.stringify(data),
      });

      if (response.ok) {
        toast.success(editingChannel ? "Channel更新成功" : "Channel创建成功");
        setIsDialogOpen(false);
        setEditingChannel(null);
        form.reset();
        fetchChannels();
      } else {
        const error = await response.json();
        toast.error(error.message || "操作失败");
      }
    } catch (error) {
      toast.error("网络错误");
    }
  };

  const handleEdit = (channel: Channel) => {
    setEditingChannel(channel);
    form.reset({
      name: channel.name,
      description: channel.description,
      provider_id: channel.provider_id,
      supported_protocols: channel.supported_protocols,
      load_balance_strategy: channel.load_balance_strategy,
      weight: channel.weight,
      status: channel.status,
    });
    setIsDialogOpen(true);
  };

  const handleDelete = async (id: number) => {
    try {
      const response = await fetch(`/api/channels/${id}`, { method: "DELETE" });
      if (response.ok) {
        toast.success("Channel删除成功");
        fetchChannels();
      } else {
        const error = await response.json();
        toast.error(error.message || "删除失败");
      }
    } catch (error) {
      toast.error("网络错误");
    }
  };

  const handleResetCooldown = async (id: number) => {
    try {
      const response = await fetch(`/api/channels/${id}/reset-cooldown`, { method: "POST" });
      if (response.ok) {
        toast.success("冷却状态重置成功");
        fetchChannels();
      } else {
        const error = await response.json();
        toast.error(error.message || "重置失败");
      }
    } catch (error) {
      toast.error("网络错误");
    }
  };

  const getStatusBadge = (status: string, cooldownUntil?: string) => {
    if (cooldownUntil && new Date(cooldownUntil) > new Date()) {
      return <Badge variant="destructive">冷却中</Badge>;
    }
    switch (status) {
      case "active":
        return <Badge variant="default">活跃</Badge>;
      case "inactive":
        return <Badge variant="secondary">停用</Badge>;
      default:
        return <Badge variant="outline">{status}</Badge>;
    }
  };

  if (loading) {
    return <div className="flex justify-center items-center h-64">加载中...</div>;
  }

  return (
    <div className="space-y-6">
      <div className="flex justify-between items-center">
        <div>
          <h1 className="text-3xl font-bold">Channel管理</h1>
          <p className="text-muted-foreground">管理统一Provider系统的渠道配置</p>
        </div>
        <Button onClick={() => setIsDialogOpen(true)}>
          <Plus className="mr-2 h-4 w-4" />
          创建Channel
        </Button>
      </div>

      <div className="grid grid-cols-1 md:grid-cols-4 gap-4">
        <Card>
          <CardHeader className="flex flex-row items-center justify-between space-y-0 pb-2">
            <CardTitle className="text-sm font-medium">总Channel数</CardTitle>
            <Activity className="h-4 w-4 text-muted-foreground" />
          </CardHeader>
          <CardContent>
            <div className="text-2xl font-bold">{channels.length}</div>
          </CardContent>
        </Card>
        <Card>
          <CardHeader className="flex flex-row items-center justify-between space-y-0 pb-2">
            <CardTitle className="text-sm font-medium">活跃Channel</CardTitle>
            <Activity className="h-4 w-4 text-green-600" />
          </CardHeader>
          <CardContent>
            <div className="text-2xl font-bold">
              {channels.filter(c => c.status === "active").length}
            </div>
          </CardContent>
        </Card>
        <Card>
          <CardHeader className="flex flex-row items-center justify-between space-y-0 pb-2">
            <CardTitle className="text-sm font-medium">总请求数</CardTitle>
            <Activity className="h-4 w-4 text-blue-600" />
          </CardHeader>
          <CardContent>
            <div className="text-2xl font-bold">
              {channels.reduce((sum, c) => sum + c.total_requests, 0)}
            </div>
          </CardContent>
        </Card>
        <Card>
          <CardHeader className="flex flex-row items-center justify-between space-y-0 pb-2">
            <CardTitle className="text-sm font-medium">平均成功率</CardTitle>
            <Activity className="h-4 w-4 text-green-600" />
          </CardHeader>
          <CardContent>
            <div className="text-2xl font-bold">
              {channels.length > 0
                ? (channels.reduce((sum, c) => sum + c.success_rate, 0) / channels.length * 100).toFixed(1)
                : 0}%
            </div>
          </CardContent>
        </Card>
      </div>

      <Card>
        <CardHeader>
          <CardTitle>Channel列表</CardTitle>
          <CardDescription>管理所有Channel的配置和状态</CardDescription>
        </CardHeader>
        <CardContent>
          <Table>
            <TableHeader>
              <TableRow>
                <TableHead>名称</TableHead>
                <TableHead>Provider</TableHead>
                <TableHead>协议</TableHead>
                <TableHead>负载均衡</TableHead>
                <TableHead>状态</TableHead>
                <TableHead>成功率</TableHead>
                <TableHead>平均响应时间</TableHead>
                <TableHead>操作</TableHead>
              </TableRow>
            </TableHeader>
            <TableBody>
              {channels.map((channel) => (
                <TableRow key={channel.id}>
                  <TableCell className="font-medium">{channel.name}</TableCell>
                  <TableCell>{channel.provider_name}</TableCell>
                  <TableCell>
                    <div className="flex gap-1">
                      {channel.supported_protocols.map((protocol) => (
                        <Badge key={protocol} variant="outline" className="text-xs">
                          {protocol}
                        </Badge>
                      ))}
                    </div>
                  </TableCell>
                  <TableCell>
                    <Badge variant="secondary">
                      {balancerTypes.find(b => b.type === channel.load_balance_strategy)?.name || channel.load_balance_strategy}
                    </Badge>
                  </TableCell>
                  <TableCell>{getStatusBadge(channel.status, channel.cooldown_until)}</TableCell>
                  <TableCell>
                    <span className={channel.success_rate > 0.9 ? "text-green-600" : channel.success_rate > 0.7 ? "text-yellow-600" : "text-red-600"}>
                      {(channel.success_rate * 100).toFixed(1)}%
                    </span>
                  </TableCell>
                  <TableCell>{channel.avg_response_time}ms</TableCell>
                  <TableCell>
                    <div className="flex gap-2">
                      <Button
                        variant="outline"
                        size="sm"
                        onClick={() => handleEdit(channel)}
                      >
                        <Edit className="h-4 w-4" />
                      </Button>
                      {channel.cooldown_until && new Date(channel.cooldown_until) > new Date() && (
                        <Button
                          variant="outline"
                          size="sm"
                          onClick={() => handleResetCooldown(channel.id)}
                        >
                          <RotateCcw className="h-4 w-4" />
                        </Button>
                      )}
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
                              确定要删除Channel "{channel.name}" 吗？此操作不可撤销。
                            </AlertDialogDescription>
                          </AlertDialogHeader>
                          <AlertDialogFooter>
                            <AlertDialogCancel>取消</AlertDialogCancel>
                            <AlertDialogAction onClick={() => handleDelete(channel.id)}>
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
            <DialogTitle>{editingChannel ? "编辑Channel" : "创建Channel"}</DialogTitle>
            <DialogDescription>
              配置Channel的基本信息和负载均衡策略
            </DialogDescription>
          </DialogHeader>
          <Form {...form}>
            <form onSubmit={form.handleSubmit(handleSubmit)} className="space-y-4">
              <FormField
                control={form.control}
                name="name"
                render={({ field }) => (
                  <FormItem>
                    <FormLabel>名称</FormLabel>
                    <FormControl>
                      <Input placeholder="输入Channel名称" {...field} />
                    </FormControl>
                    <FormMessage />
                  </FormItem>
                )}
              />
              <FormField
                control={form.control}
                name="description"
                render={({ field }) => (
                  <FormItem>
                    <FormLabel>描述</FormLabel>
                    <FormControl>
                      <Textarea placeholder="输入Channel描述" {...field} />
                    </FormControl>
                    <FormMessage />
                  </FormItem>
                )}
              />
              <FormField
                control={form.control}
                name="provider_id"
                render={({ field }) => (
                  <FormItem>
                    <FormLabel>Provider</FormLabel>
                    <Select onValueChange={(value) => field.onChange(parseInt(value))} value={field.value?.toString()}>
                      <FormControl>
                        <SelectTrigger>
                          <SelectValue placeholder="选择Provider" />
                        </SelectTrigger>
                      </FormControl>
                      <SelectContent>
                        {providers.map((provider) => (
                          <SelectItem key={provider.id} value={provider.id.toString()}>
                            {provider.name} ({provider.type})
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
                name="load_balance_strategy"
                render={({ field }) => (
                  <FormItem>
                    <FormLabel>负载均衡策略</FormLabel>
                    <Select onValueChange={field.onChange} value={field.value}>
                      <FormControl>
                        <SelectTrigger>
                          <SelectValue placeholder="选择负载均衡策略" />
                        </SelectTrigger>
                      </FormControl>
                      <SelectContent>
                        {balancerTypes.map((type) => (
                          <SelectItem key={type.type} value={type.type}>
                            {type.name} - {type.description}
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
                  {editingChannel ? "更新" : "创建"}
                </Button>
              </DialogFooter>
            </form>
          </Form>
        </DialogContent>
      </Dialog>
    </div>
  );
}