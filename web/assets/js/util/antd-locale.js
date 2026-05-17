(function (global) {
    if (typeof moment !== 'undefined') {
        moment.locale('zh-cn');
    }
    if (typeof antd !== 'undefined' && antd.locales) {
        global.antdLocale = antd.locales.zh_CN || antd.locales['zh_CN'];
    } else {
        global.antdLocale = null;
    }
    global.datePickerLocale = (global.antdLocale && global.antdLocale.DatePicker)
        ? global.antdLocale.DatePicker
        : {};
})(window);
