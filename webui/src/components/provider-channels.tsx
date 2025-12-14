import { useState, useEffect } from "react";
import { Badge } from "@/components/ui/badge";
import { Button } from "@/components/ui/button";
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogHeader,
  DialogTitle,
  DialogTrigger,
} from "@/components/ui/dialog";
import {
  Table,
  TableBody,
  TableCell,
  TableHead,
  TableHeader,
  TableRow
} from "@/components/ui/table";
import { Card, CardContent, CardHeader, CardTitle } from "@/components/ui/card";
import { FaNetworkWired } from "react-icons/fa";
import { toast } from "sonner";

interface Channel {
  id: number;
  name: string;
  description: string;
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
}

interface ProviderChannelsProps {
  providerId: number;
  providerName: string;
}

export default function ProviderChannels({ providerId, providerName }: ProviderChannelsProps) {
  const [channels, setChannels] = useState<Channel[]>([]);
  const [loading, setLoading] = useState(false);
  const [open, setOpen] = useState(false);

  const fetchChannels = async () => {
    if (!open) return;

    try {
      setLoading(true);
      const response = await fetch(`/api/channels?provider_id=${providerId}`);
      if (response.ok) {
        const data = await response.json();
        setChannels(data.data || []);
      } else {
        toast.error("获取Channel列表失败");
      }
    } catch (error) {
      toast.error("网络错误");
    } finally {
      setLoading(false);
    }
  };

  useEffect(() => {
    fetchChannels();
  }, [open, providerId]);

  const getStatusBadge = (status: string) => {
    switch (status) {
      case "active":
        return <Badge variant="default">活跃</Badge>;
      case "inactive":
        return <Badge variant="secondary">停用</Badge>;
      case "cooldown":
        return <Badge variant="destructive">冷却中</Badge>;
      default:
        return <Badge variant="outline">{status}</Badge>;
    }
  };

  const getBalancerName = (strategy: string) => {
    const names: Record<string, string> = {
      lottery: "权重抽签",
      rotor: "循环轮转",
      smooth_weighted_rr: "平滑加权轮询",
      error_aware: "错误感知",
      trace_aware: "响应时间感知",
      weight_round_robin: "加权轮询",
      connection_aware: "连接数感知",
    };
    return names[strategy] || strategy;
  };

  return (
    <Dialog open={open} onOpenChange={setOpen}>
      <DialogTrigger asChild>
        <Button variant="outline" size="sm" title={`查看 ${providerName} 的Channel`}>
          <FaNetworkWired className="mr-1 h-3 w-3" />
          Channel
        </Button>
      </DialogTrigger>
      <DialogContent className="max-w-4xl max-h-[80vh] overflow-y-auto">
        <DialogHeader>
          <DialogTitle className="flex items-center gap-2">
            <FaNetworkWired />
            {providerName} - Channel列表
          </DialogTitle>
          <DialogDescription>
            查看该Provider关联的所有Channel配置和统计信息
          </DialogDescription>
        </DialogHeader>

        {loading ? (
          <div className="flex justify-center items-center h-32">
            <div className="text-muted-foreground">加载中...</div>
          </div>
        ) : channels.length === 0 ? (
          <Card>
            <CardContent className="flex flex-col items-center justify-center py-8">
              <FaNetworkWired className="h-12 w-12 text-muted-foreground mb-4" />
              <p className="text-muted-foreground">该Provider暂无关联的Channel</p>
              <p className="text-sm text-muted-foreground mt-2">
                您可以在Channel管理页面创建新的Channel
              </p>
            </CardContent>
          </Card>
        ) : (
          <div className="space-y-4">
            <div className="grid grid-cols-1 md:grid-cols-3 gap-4">
              <Card>
                <CardHeader className="flex flex-row items-center justify-between space-y-0 pb-2">
                  <CardTitle className="text-sm font-medium">总Channel数</CardTitle>
                </CardHeader>
                <CardContent>
                  <div className="text-2xl font-bold">{channels.length}</div>
                </CardContent>
              </Card>
              <Card>
                <CardHeader className="flex flex-row items-center justify-between space-y-0 pb-2">
                  <CardTitle className="text-sm font-medium">活跃Channel</CardTitle>
                </CardHeader>
                <CardContent>
                  <div className="text-2xl font-bold text-green-600">
                    {channels.filter(c => c.status === "active").length}
                  </div>
                </CardContent>
              </Card>
              <Card>
                <CardHeader className="flex flex-row items-center justify-between space-y-0 pb-2">
                  <CardTitle className="text-sm font-medium">总请求数</CardTitle>
                </CardHeader>
                <CardContent>
                  <div className="text-2xl font-bold">
                    {channels.reduce((sum, c) => sum + c.total_requests, 0)}
                  </div>
                </CardContent>
              </Card>
            </div>

            <Card>
              <CardHeader>
                <CardTitle>Channel详情</CardTitle>
              </CardHeader>
              <CardContent>
                <Table>
                  <TableHeader>
                    <TableRow>
                      <TableHead>名称</TableHead>
                      <TableHead>协议</TableHead>
                      <TableHead>负载均衡</TableHead>
                      <TableHead>状态</TableHead>
                      <TableHead>成功率</TableHead>
                      <TableHead>响应时间</TableHead>
                    </TableRow>
                  </TableHeader>
                  <TableBody>
                    {channels.map((channel) => (
                      <TableRow key={channel.id}>
                        <TableCell>
                          <div>
                            <div className="font-medium">{channel.name}</div>
                            {channel.description && (
                              <div className="text-sm text-muted-foreground">
                                {channel.description}
                              </div>
                            )}
                          </div>
                        </TableCell>
                        <TableCell>
                          <div className="flex gap-1 flex-wrap">
                            {channel.supported_protocols.map((protocol) => (
                              <Badge key={protocol} variant="outline" className="text-xs">
                                {protocol}
                              </Badge>
                            ))}
                          </div>
                        </TableCell>
                        <TableCell>
                          <Badge variant="secondary" className="text-xs">
                            {getBalancerName(channel.load_balance_strategy)}
                          </Badge>
                        </TableCell>
                        <TableCell>{getStatusBadge(channel.status)}</TableCell>
                        <TableCell>
                          <span className={
                            channel.success_rate > 0.9
                              ? "text-green-600"
                              : channel.success_rate > 0.7
                                ? "text-yellow-600"
                                : "text-red-600"
                          }>
                            {(channel.success_rate * 100).toFixed(1)}%
                          </span>
                        </TableCell>
                        <TableCell>{channel.avg_response_time}ms</TableCell>
                      </TableRow>
                    ))}
                  </TableBody>
                </Table>
              </CardContent>
            </Card>
          </div>
        )}
      </DialogContent>
    </Dialog>
  );
}