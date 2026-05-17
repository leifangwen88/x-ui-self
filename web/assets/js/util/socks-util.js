class SocksUtil {

    /** 批量导入默认到期：当前时间 + 1 个月 */
    static defaultImportExpiry() {
        return moment().add(1, 'month').seconds(0).milliseconds(0);
    }

    /** socksProxyId -> inboundId */
    static buildBoundMap(inbounds) {
        const map = {};
        if (!inbounds) {
            return map;
        }
        for (const ib of inbounds) {
            const socksId = ib.socksProxyId || 0;
            if (socksId > 0) {
                map[socksId] = ib.id;
            }
        }
        return map;
    }

    /**
     * 可选入站：未绑定 SOCKS、或已绑定到当前这条 SOCKS（编辑时保留当前项）
     */
    static filterSelectableInbounds(inbounds, socksId) {
        if (!inbounds || inbounds.length === 0) {
            return [];
        }
        const currentSocks = socksId || 0;
        return inbounds.filter(ib => {
            const sid = ib.socksProxyId || 0;
            if (sid === 0) {
                return true;
            }
            return currentSocks > 0 && sid === currentSocks;
        });
    }

    /** 入站下拉：备注优先 */
    static formatInboundOptionLabel(ib) {
        if (!ib) {
            return '';
        }
        const meta = ['#' + ib.id, '端口 ' + ib.port, ib.protocol];
        const remark = (ib.remark || '').trim();
        if (remark) {
            return remark + '（' + meta.join(' · ') + '）';
        }
        return meta.join(' · ');
    }

    /**
     * 可选 SOCKS：未绑定、或仅绑定到当前入站（编辑时保留当前项）
     */
    static filterSelectable(socksList, inbounds, inboundId, currentSocksProxyId) {
        if (!socksList || socksList.length === 0) {
            return [];
        }
        const boundMap = SocksUtil.buildBoundMap(inbounds);
        const currentId = inboundId || 0;
        const currentSocks = currentSocksProxyId || 0;
        return socksList.filter(s => {
            if (s.enable === false) {
                return false;
            }
            const ownerId = boundMap[s.id];
            if (!ownerId) {
                return true;
            }
            if (currentId > 0 && ownerId === currentId) {
                return true;
            }
            if (currentSocks > 0 && s.id === currentSocks && currentId > 0 && ownerId === currentId) {
                return true;
            }
            return false;
        });
    }

    static isExpired(s) {
        return s && s.expiryTime > 0 && s.expiryTime < Date.now();
    }

    /** 有备注时优先显示备注名 */
    static formatOptionLabel(s) {
        if (!s) {
            return '';
        }
        const meta = [];
        meta.push('#' + s.id);
        meta.push(s.address + ':' + s.port);
        if (s.expiryTime > 0) {
            meta.push(SocksUtil.isExpired(s) ? '已到期' : DateUtil.formatMillis(s.expiryTime));
        } else {
            meta.push('无期限');
        }
        const remark = (s.remark || '').trim();
        if (remark) {
            return remark + '（' + meta.join(' · ') + '）';
        }
        return meta.join(' · ');
    }
}
