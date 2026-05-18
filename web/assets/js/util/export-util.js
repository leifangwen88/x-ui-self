/**
 * 入站批量导出：分享链接、V2Ray / 通用订阅 Base64、Clash YAML
 */
class ExportUtil {

    /** 批量导出：全部已启用且支持链接的入站（与列表勾选无关） */
    static collect(dbInbounds) {
        return (dbInbounds || []).filter(ib => ib.enable && ib.hasLink());
    }

    static proxyName(dbInbound) {
        const n = (dbInbound.remark || ('inbound-' + dbInbound.port)).trim();
        return n.replace(/[\r\n]/g, ' ').slice(0, 64);
    }

    static resolveServer(dbInbound, inbound) {
        let address = dbInbound.address;
        const stream = inbound.stream;
        if ((stream.isTls || stream.isXTls) && !ObjectUtil.isEmpty(stream.tls.server)) {
            address = stream.tls.server;
        }
        return address;
    }

    static genShareLinks(dbInbounds) {
        return this.collect(dbInbounds).map(ib => ib.genLink());
    }

    /** v2rayNG / v2rayN / Nekoray 等：仅 vmess/vless/trojan/ss 链接的 Base64 订阅 */
    static genV2raySubscriptionBase64(dbInbounds) {
        const links = this.filterV2rayShareLinks(this.genShareLinks(dbInbounds));
        if (links.length === 0) {
            return '';
        }
        return base64(links.join('\n'));
    }

    static filterV2rayShareLinks(links) {
        return (links || []).filter(l => {
            const s = String(l || '').trim().toLowerCase();
            return s.startsWith('vmess://') || s.startsWith('vless://')
                || s.startsWith('trojan://') || s.startsWith('ss://');
        });
    }

    /** 小火箭 / Surge / QX 等常用的 Base64 订阅正文 */
    static genSubscriptionBase64(dbInbounds) {
        const links = this.genShareLinks(dbInbounds);
        if (links.length === 0) {
            return '';
        }
        return base64(links.join('\n'));
    }

    static yamlQuote(str) {
        if (str == null) {
            return '""';
        }
        const s = String(str);
        if (/[:#\n\r\t'"{}[\]&*!|>%@`,]/.test(s) || s === '' || s !== s.trim()) {
            return JSON.stringify(s);
        }
        return s;
    }

    static yamlLine(indent, key, value, quoted = false) {
        const pad = '  '.repeat(indent);
        if (value === undefined || value === null) {
            return '';
        }
        if (typeof value === 'boolean') {
            return `${pad}${key}: ${value}\n`;
        }
        if (typeof value === 'number') {
            return `${pad}${key}: ${value}\n`;
        }
        const v = quoted ? this.yamlQuote(value) : value;
        return `${pad}${key}: ${v}\n`;
    }

    static appendNetworkOpts(lines, indent, stream) {
        const net = stream.network;
        if (net === 'ws') {
            lines.push(`${'  '.repeat(indent)}ws-opts:`);
            lines.push(this.yamlLine(indent + 1, 'path', stream.ws.path || '/', true));
            const hostIdx = stream.ws.headers.findIndex(h => h.name.toLowerCase() === 'host');
            if (hostIdx >= 0 && stream.ws.headers[hostIdx].value) {
                lines.push(`${'  '.repeat(indent + 1)}headers:`);
                lines.push(this.yamlLine(indent + 2, 'Host', stream.ws.headers[hostIdx].value, true));
            }
        } else if (net === 'grpc') {
            lines.push(`${'  '.repeat(indent)}grpc-opts:`);
            lines.push(this.yamlLine(indent + 1, 'grpc-service-name', stream.grpc.serviceName || '', true));
        } else if (net === 'http') {
            lines.push(`${'  '.repeat(indent)}network: h2`);
            lines.push(`${'  '.repeat(indent)}h2-opts:`);
            const path = Array.isArray(stream.http.path) ? stream.http.path[0] : stream.http.path;
            lines.push(this.yamlLine(indent + 1, 'path', path || '/', true));
            const hosts = stream.http.host;
            if (hosts && hosts.length) {
                lines.push(`${'  '.repeat(indent + 1)}host:`);
                for (const h of hosts) {
                    lines.push(`${'  '.repeat(indent + 2)}- ${this.yamlQuote(h)}`);
                }
                lines.push('');
            }
        } else if (net === 'tcp' && stream.tcp.type === 'http') {
            const req = stream.tcp.request;
            lines.push(`${'  '.repeat(indent)}network: http`);
            lines.push(`${'  '.repeat(indent)}http-opts:`);
            if (req.path && req.path.length) {
                lines.push(`${'  '.repeat(indent + 1)}path:`);
                for (const p of req.path) {
                    lines.push(`${'  '.repeat(indent + 2)}- ${this.yamlQuote(p)}`);
                }
            }
            const hostIdx = req.headers.findIndex(h => h.name.toLowerCase() === 'host');
            if (hostIdx >= 0) {
                lines.push(`${'  '.repeat(indent + 1)}headers:`);
                lines.push(this.yamlLine(indent + 2, 'Host', req.headers[hostIdx].value, true));
            }
            lines.push('');
        }
    }

    static appendTls(lines, indent, stream) {
        if (stream.isTls) {
            this.pushLine(lines, indent, 'tls', true);
            if (!ObjectUtil.isEmpty(stream.tls.server)) {
                this.pushLine(lines, indent, 'servername', stream.tls.server, true);
            }
            this.pushLine(lines, indent, 'skip-cert-verify', true);
        } else if (stream.isXTls) {
            this.pushLine(lines, indent, 'tls', true);
            this.pushLine(lines, indent, 'skip-cert-verify', true);
            if (!ObjectUtil.isEmpty(stream.tls.server)) {
                this.pushLine(lines, indent, 'servername', stream.tls.server, true);
            }
        }
    }

    static pushLine(lines, indent, key, value, quoted) {
        const line = this.yamlLine(indent, key, value, quoted);
        if (line) {
            lines.push(line.replace(/\n$/, ''));
        }
    }

    static toClashProxyLines(dbInbound) {
        const inbound = dbInbound.toInbound();
        const stream = inbound.stream;
        const name = this.proxyName(dbInbound);
        const server = this.resolveServer(dbInbound, inbound);
        const port = inbound.port;
        const lines = [];
        const push = (k, v, q) => this.pushLine(lines, 0, k, v, q);

        switch (inbound.protocol) {
            case Protocols.VMESS: {
                const vm = inbound.settings.vmesses[0];
                push('name', name, true);
                push('type', 'vmess', true);
                push('server', server, true);
                push('port', port);
                push('uuid', vm.id, true);
                push('alterId', vm.alterId);
                push('cipher', 'auto', true);
                push('udp', true);
                this.appendTls(lines, 0, stream);
                if (stream.network && stream.network !== 'tcp') {
                    push('network', stream.network, true);
                }
                this.appendNetworkOpts(lines, 0, stream);
                break;
            }
            case Protocols.VLESS: {
                const vl = inbound.settings.vlesses[0];
                push('name', name, true);
                push('type', 'vless', true);
                push('server', server, true);
                push('port', port);
                push('uuid', vl.id, true);
                push('udp', true);
                if (stream.isXTls && vl.flow) {
                    push('flow', vl.flow, true);
                }
                this.appendTls(lines, 0, stream);
                if (stream.network && stream.network !== 'tcp') {
                    push('network', stream.network, true);
                }
                this.appendNetworkOpts(lines, 0, stream);
                break;
            }
            case Protocols.TROJAN: {
                const pw = inbound.settings.clients[0].password;
                push('name', name, true);
                push('type', 'trojan', true);
                push('server', server, true);
                push('port', port);
                push('password', pw, true);
                push('udp', true);
                this.appendTls(lines, 0, stream);
                break;
            }
            case Protocols.SHADOWSOCKS: {
                const ss = inbound.settings;
                push('name', name, true);
                push('type', 'ss', true);
                push('server', server, true);
                push('port', port);
                push('cipher', ss.method, true);
                push('password', ss.password, true);
                push('udp', true);
                break;
            }
            default:
                return null;
        }
        return lines;
    }

    static genClashYaml(dbInbounds) {
        const list = this.collect(dbInbounds);
        const names = [];
        const skipped = [];
        const proxyYaml = [];

        for (const ib of list) {
            const lines = this.toClashProxyLines(ib);
            if (!lines || !lines.length) {
                skipped.push(ib.remark || ib.port);
                continue;
            }
            names.push(this.proxyName(ib));
            proxyYaml.push('  -');
            for (const line of lines) {
                proxyYaml.push('    ' + line);
            }
        }

        if (names.length === 0) {
            return { yaml: '', skipped: skipped, count: 0 };
        }

        let yaml = '# Clash / Mihomo 配置（由 x-ui 批量导出）\n'
            + '# 苹果客户端 V2Box、Karing 等可直接使用本订阅\n';
        yaml += 'proxies:\n';
        yaml += proxyYaml.join('\n') + '\n';
        yaml += '\nproxy-groups:\n';
        yaml += '  - name: ' + this.yamlQuote('节点选择') + '\n';
        yaml += '    type: select\n';
        yaml += '    proxies:\n';
        for (const n of names) {
            yaml += '      - ' + this.yamlQuote(n) + '\n';
        }
        yaml += '      - DIRECT\n';
        yaml += '\nrules:\n';
        yaml += '  - MATCH,节点选择\n';

        return { yaml: yaml, skipped: skipped, count: names.length };
    }

    static subscriptionHint(basePath) {
        const base = (basePath || '/').replace(/\/?$/, '/');
        return '【订阅说明】\n'
            + '1. 推荐使用入站列表页顶部的「订阅地址」自动 URL。\n'
            + '2. v2rayNG / v2rayN 请用「V2Ray 订阅」URL；小火箭 / Surge 可用「通用订阅」。\n'
            + '3. 下方为离线 Base64 正文，也可保存为 .txt 放到 HTTPS 静态地址后当订阅用。\n'
            + '4. 面板路径前缀：' + base + '\n';
    }

    static v2rayJsonHint(singleUrl, clusterUrl, basePath) {
        const base = (basePath || '/').replace(/\/?$/, '/');
        let hint = '【V2Ray JSON 说明 · V2Box】\n'
            + '1. 在 V2Box 中选择从 URL / 剪贴板导入配置（非「订阅链接」）。\n'
            + '2. 粘贴下方单站或站群 JSON 订阅 URL，客户端会拉取完整 JSON（含 balancer 与游戏分组）。\n'
            + '3. 默认路由走「单站」或「站群负载均衡」；可在出站/负载均衡列表切换游戏或入口组。\n';
        if (singleUrl) {
            hint += '\n单站 JSON URL：\n' + singleUrl + '\n';
        }
        if (clusterUrl) {
            hint += '\n站群 JSON URL：\n' + clusterUrl + '\n';
        }
        if (!singleUrl) {
            hint += '\n请先在入站页展开「订阅地址」复制 V2Ray JSON URL。\n';
        }
        hint += '\n面板路径前缀：' + base + '\n';
        return hint;
    }

    static v2raySubscriptionHint(subUrl, basePath) {
        const base = (basePath || '/').replace(/\/?$/, '/');
        let hint = '【V2Ray 订阅说明】\n'
            + '1. v2rayNG / v2rayN / Nekoray：在客户端添加「订阅」，粘贴下方 URL 或 Base64 正文。\n';
        if (subUrl) {
            hint += '2. 推荐订阅 URL：\n' + subUrl + '\n';
            hint += '3. 面板路径前缀：' + base + '\n';
        } else {
            hint += '2. 请先在入站页展开「订阅地址」复制「V2Ray 订阅」URL。\n'
                + '3. 面板路径前缀：' + base + '\n';
        }
        return hint;
    }
}
