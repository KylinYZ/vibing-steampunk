FUNCTION ZVSP_COMPAT_751
  IMPORTING
    VALUE(i_op) TYPE char20
    VALUE(i_program) TYPE programm OPTIONAL
    VALUE(i_language) TYPE sylangu DEFAULT sy-langu
    VALUE(i_devclass) TYPE devclass OPTIONAL
    VALUE(i_transport) TYPE trkorr OPTIONAL
    VALUE(i_table) TYPE rpy_tabl-tablname OPTIONAL
    VALUE(i_description) TYPE ddtext OPTIONAL
    VALUE(i_delivery_class) TYPE char1 OPTIONAL
    VALUE(i_table_category) TYPE char8 OPTIONAL
    VALUE(i_tabart) TYPE char5 OPTIONAL
    VALUE(i_buffering) TYPE char1 OPTIONAL
    VALUE(i_fields_json) TYPE string OPTIONAL
  EXPORTING
    VALUE(e_rc) TYPE i
    VALUE(e_activation_rc) TYPE i
    VALUE(e_message) TYPE string
  TABLES
    i_textpool STRUCTURE textpool OPTIONAL
    e_textpool STRUCTURE textpool OPTIONAL
.

************************************************************************
* ZVSP_COMPAT_751 -- SAP NetWeaver 7.51 兼容 RFC 门面。
*
* 背景：ADT 的"数据库表创建/源码编辑"与"文本元素"资源自 7.52 SP00 才
* 提供。在 7.51 上 VSP 的 CreateTable / WriteTextPool 会收到 404。本函数
* 模块以封闭动作集补齐这两类能力，内部调用 SE11 同款标准仓库 API：
*
*   TEXTPOOL_GET   READ TEXTPOOL            —— 读程序完整文本池
*   TEXTPOOL_SET   RPY_TEXTELEMENTS_INSERT  —— 以完整 TEXTPOOL 覆盖写入
*   TABLE_CREATE   RPY_TABLE_INSERT + DDIF_TABL_ACTIVATE
*                                           —— 创建并激活透明表
*
* 安全模型：
*   - 只接受 Z/Y 对象名；不是通用 CALL FUNCTION 代理。
*   - 可传输包必须带传输请求号（本地包 $ 开头除外）。
*   - 激活授权检查（AUTH_CHK='X'）永不关闭。
*
* SE37 部署：按上述接口逐个建参数，处理类型选"远程可调用的模块"
* (Remote-Enabled Module)，然后激活。全部逻辑内联在本函数体内，
* 函数组里不需要任何额外的 FORM/全局数据。
*
* 字段定义协议（TABLE_CREATE，I_FIELDS_JSON）：
*   字段数组 JSON，每个元素：
*     name         字段名（必填，<=30 字符）
*     type         DDIC 内置类型码：CLNT/CHAR/NUMC/RAW/DEC/CURR/QUAN/
*                  INT1/INT2/INT4/INT8/FLTP/STRING/RAWSTRING/DATS/TIMS/
*                  LANG/CUKY/UNIT；别名 DATE/TIME/CLIENT/MANDT 会归一化
*     dataElement  数据元素名；与 type 二选一，给出时优先
*     len          长度（CHAR/NUMC/RAW/DEC/CURR/QUAN/CLNT）
*     dec          小数位（DEC/CURR/QUAN）
*     key          true = 主键字段（隐含 NOT NULL）
*     notNull      true = NOT NULL（初始值强制）
*     description  字段描述
*   客户端字段（MANDT）由调用方放入数组，本函数不自动添加。
*
* 失败约定：e_rc <> 0 即失败，e_message 带原因。TABLE_CREATE 里
* RPY_TABLE_INSERT 成功而激活失败时 e_rc=12、e_activation_rc>4，
* 会留下未激活的 DDIC 定义——需要人工在 SE11 处理，本函数不做删除。
************************************************************************

  DATA lv_name TYPE programm.
  DATA lv_text TYPE string.
  DATA lv_op TYPE char20.

  CLEAR: e_rc, e_activation_rc, e_message.
  lv_op = i_op.
  TRANSLATE lv_op TO UPPER CASE.

  CASE lv_op.

    WHEN 'TEXTPOOL_GET'.
      lv_name = i_program.
      IF lv_name IS INITIAL OR ( lv_name(1) <> 'Z' AND lv_name(1) <> 'Y' ).
        e_rc = 8.
        e_message = 'Only Z/Y programs are allowed'.
        RETURN.
      ENDIF.

      READ TEXTPOOL lv_name LANGUAGE i_language INTO e_textpool.
      e_rc = sy-subrc.
      IF e_rc <> 0.
        " 不存在的程序与空池都返回非零；消息里带上对象名便于定位。
        e_message = |READ TEXTPOOL failed for { lv_name } ({ i_language })|.
      ENDIF.

    WHEN 'TEXTPOOL_SET'.
      lv_name = i_program.
      IF lv_name IS INITIAL OR ( lv_name(1) <> 'Z' AND lv_name(1) <> 'Y' ).
        e_rc = 8.
        e_message = 'Only Z/Y programs are allowed'.
        RETURN.
      ENDIF.
      IF i_textpool[] IS INITIAL.
        e_rc = 8.
        e_message = 'I_TEXTPOOL is required for TEXTPOOL_SET'.
        RETURN.
      ENDIF.
      IF i_devclass IS INITIAL.
        e_rc = 8.
        e_message = 'I_DEVCLASS is required for TEXTPOOL_SET'.
        RETURN.
      ENDIF.
      IF i_devclass(1) <> '$' AND i_transport IS INITIAL.
        e_rc = 8.
        e_message = 'I_TRANSPORT is required for a transportable program'.
        RETURN.
      ENDIF.

      " RPY_TEXTELEMENTS_INSERT 是 SE38 文本元素维护的标准 API：
      " SOURCE 是完整 TEXTPOOL（不是补丁），suppress_dialog 关掉所有弹窗，
      " 传输归属由 transport_number 显式给出。
      CALL FUNCTION 'RPY_TEXTELEMENTS_INSERT'
        EXPORTING
          development_class  = i_devclass
          language           = i_language
          program_name       = lv_name
          transport_number   = i_transport
          suppress_dialog    = 'X'
        TABLES
          source             = i_textpool
        EXCEPTIONS
          cancelled          = 1
          permission_error   = 2
          program_not_exists = 3
          OTHERS             = 4.
      e_rc = sy-subrc.
      IF e_rc <> 0.
        MESSAGE ID sy-msgid TYPE 'S' NUMBER sy-msgno
          WITH sy-msgv1 sy-msgv2 sy-msgv3 sy-msgv4
          INTO lv_text.
        e_message = lv_text.
        RETURN.
      ENDIF.

      " 写入后读回核验：把活动版本的完整池放回 E_TEXTPOOL，
      " 条数写进消息，调用方据此确认写入真的生效。
      CLEAR e_textpool[].
      READ TEXTPOOL lv_name LANGUAGE i_language INTO e_textpool.
      e_message = |written; { lines( e_textpool[] ) } entries read back|.

    WHEN 'TABLE_CREATE'.
      IF i_table IS INITIAL OR ( i_table(1) <> 'Z' AND i_table(1) <> 'Y' ).
        e_rc = 8.
        e_message = 'Only Z/Y tables are allowed'.
        RETURN.
      ENDIF.
      IF i_devclass IS INITIAL.
        e_rc = 8.
        e_message = 'I_DEVCLASS is required for TABLE_CREATE'.
        RETURN.
      ENDIF.
      IF i_devclass(1) <> '$' AND i_transport IS INITIAL.
        e_rc = 8.
        e_message = 'I_TRANSPORT is required for a transportable table'.
        RETURN.
      ENDIF.
      IF i_fields_json IS INITIAL.
        e_rc = 8.
        e_message = 'I_FIELDS_JSON must describe at least one field'.
        RETURN.
      ENDIF.

      " --- 解析字段 JSON 并组装 RPY_FIEL_U 行 ---
      DATA lt_fields TYPE STANDARD TABLE OF rpy_fiel_u WITH DEFAULT KEY.
      DATA ls_field TYPE rpy_fiel_u.
      DATA lr_fields TYPE REF TO data.
      DATA lv_len TYPE i.
      DATA lv_dec TYPE i.
      DATA lv_fname TYPE string.
      FIELD-SYMBOLS <lt_fields> TYPE ANY TABLE.
      FIELD-SYMBOLS <ls_field_json> TYPE any.
      FIELD-SYMBOLS <lv_val> TYPE any.

      lr_fields = /ui2/cl_json=>generate( json = i_fields_json ).
      IF lr_fields IS NOT BOUND.
        e_rc = 8.
        e_message = 'I_FIELDS_JSON is not valid JSON'.
        RETURN.
      ENDIF.
      ASSIGN lr_fields->* TO <lt_fields>.
      IF sy-subrc <> 0.
        e_rc = 8.
        e_message = 'I_FIELDS_JSON must be a JSON array of field objects'.
        RETURN.
      ENDIF.

      LOOP AT <lt_fields> ASSIGNING <ls_field_json>.
        CLEAR: ls_field, lv_len, lv_dec, lv_fname.

        " 字段名必填且 <=30 字符（DDIC FIELDNAME 的宽度）。
        UNASSIGN <lv_val>.
        ASSIGN COMPONENT 'NAME' OF STRUCTURE <ls_field_json> TO <lv_val>.
        IF sy-subrc <> 0 OR <lv_val> IS INITIAL.
          e_rc = 8.
          e_message = |field { sy-tabix }: 'name' is required|.
          RETURN.
        ENDIF.
        lv_fname = <lv_val>.
        IF strlen( lv_fname ) > 30.
          e_rc = 8.
          e_message = |field { lv_fname }: name longer than 30 characters|.
          RETURN.
        ENDIF.
        ls_field-fieldname = lv_fname.

        " 描述：RPY_FIEL_U 的 DESCRIPtio 为 CHAR60，超长由 MOVE 截断。
        UNASSIGN <lv_val>.
        ASSIGN COMPONENT 'DESCRIPTION' OF STRUCTURE <ls_field_json> TO <lv_val>.
        IF sy-subrc = 0.
          ls_field-descriptio = <lv_val>.
        ENDIF.

        " 主键与 NOT NULL：主键字段在 SE11 语义里必然 NOT NULL。
        UNASSIGN <lv_val>.
        ASSIGN COMPONENT 'KEY' OF STRUCTURE <ls_field_json> TO <lv_val>.
        IF sy-subrc = 0 AND <lv_val> = 'X'.
          ls_field-keyflag = 'X'.
          ls_field-notnull = 'X'.
        ELSE.
          UNASSIGN <lv_val>.
          ASSIGN COMPONENT 'NOTNULL' OF STRUCTURE <ls_field_json> TO <lv_val>.
          IF sy-subrc = 0 AND <lv_val> = 'X'.
            ls_field-notnull = 'X'.
          ENDIF.
        ENDIF.

        " 类型：dataElement 优先（ROLLNAME）；否则 type 归一化为
        " DDIC DATATYPE 内部码。STRING/RAWSTRING 的内部码是 STRG/RSTR，
        " 与 SE11 展示值不同，这里是唯一翻译点。
        UNASSIGN <lv_val>.
        ASSIGN COMPONENT 'DATAELEMENT' OF STRUCTURE <ls_field_json> TO <lv_val>.
        IF sy-subrc = 0 AND <lv_val> IS NOT INITIAL.
          ls_field-rollname = <lv_val>.
          TRANSLATE ls_field-rollname TO UPPER CASE.
        ELSE.
          UNASSIGN <lv_val>.
          ASSIGN COMPONENT 'TYPE' OF STRUCTURE <ls_field_json> TO <lv_val>.
          IF sy-subrc <> 0 OR <lv_val> IS INITIAL.
            e_rc = 8.
            e_message = |field { lv_fname }: either 'type' or 'dataElement' is required|.
            RETURN.
          ENDIF.
          DATA lv_type TYPE string.
          lv_type = <lv_val>.
          TRANSLATE lv_type TO UPPER CASE.
          CASE lv_type.
            WHEN 'CLIENT' OR 'MANDT'.
              ls_field-datatype = 'CLNT'.
              IF lv_len = 0.
                lv_len = 3.
              ENDIF.
            WHEN 'DATE'.
              ls_field-datatype = 'DATS'.
            WHEN 'TIME'.
              ls_field-datatype = 'TIMS'.
            WHEN 'STRING'.
              ls_field-datatype = 'STRG'.
            WHEN 'RAWSTRING'.
              ls_field-datatype = 'RSTR'.
            WHEN OTHERS.
              " CLNT/CHAR/NUMC/RAW/DEC/CURR/QUAN/INT1/INT2/INT4/INT8/
              " FLTP/LANG/CUKY/UNIT 直接透传，合法性由 RPY_TABLE_INSERT
              " 校验（非法类型会以 e_rc<>0 失败，不产生半成品）。
              ls_field-datatype = lv_type.
          ENDCASE.
          " 长度与小数位：generate 生成的数字组件用通用赋值转入 i。
          UNASSIGN <lv_val>.
          ASSIGN COMPONENT 'LEN' OF STRUCTURE <ls_field_json> TO <lv_val>.
          IF sy-subrc = 0.
            lv_len = <lv_val>.
          ENDIF.
          UNASSIGN <lv_val>.
          ASSIGN COMPONENT 'DEC' OF STRUCTURE <ls_field_json> TO <lv_val>.
          IF sy-subrc = 0.
            lv_dec = <lv_val>.
          ENDIF.
          ls_field-leng = lv_len.
          ls_field-decimals = lv_dec.
        ENDIF.

        APPEND ls_field TO lt_fields.
      ENDLOOP.

      IF lt_fields IS INITIAL.
        e_rc = 8.
        e_message = 'I_FIELDS_JSON described no field'.
        RETURN.
      ENDIF.

      " 表头与缺省：交付类 A、透明表 TRANSP，与 Go 端 CreateTableOptions
      " 的缺省一致；双端都有缺省是为了任何一端直接调用也不出错。
      DATA lv_delivery TYPE rpy_tabl-dliclass.
      DATA lv_tabclass TYPE rpy_tabl-tablclass.
      lv_delivery = i_delivery_class.
      IF lv_delivery IS INITIAL.
        lv_delivery = 'A'.
      ENDIF.
      lv_tabclass = i_table_category.
      IF lv_tabclass IS INITIAL.
        lv_tabclass = 'TRANSP'.
      ENDIF.

      DATA ls_tabl TYPE rpy_tabl.
      ls_tabl-tablname   = i_table.
      ls_tabl-descriptio = i_description.
      ls_tabl-tablclass  = lv_tabclass.
      ls_tabl-dliclass   = lv_delivery.

      " 技术设置：数据类为空时取 SE11 新表缺省 APPL0；缓冲为空表示
      " 不允许缓冲；SCHFELD_ANZ 仅全缓冲时有意义，缺省 0。
      DATA ls_tech TYPE rpy_tbtech.
      ls_tech-tabart      = i_tabart.
      IF ls_tech-tabart IS INITIAL.
        ls_tech-tabart = 'APPL0'.
      ENDIF.
      ls_tech-pufferung   = i_buffering.
      ls_tech-schfeld_anz = 0.

      CALL FUNCTION 'RPY_TABLE_INSERT'
        EXPORTING
          language          = i_language
          table_name        = i_table
          transport_number  = i_transport
          development_class = i_devclass
          tabl_inf          = ls_tabl
          tabl_technics     = ls_tech
        TABLES
          tabl_fields       = lt_fields
        EXCEPTIONS
          cancelled         = 1
          already_exist     = 2
          permission_error  = 3
          name_not_allowed  = 4
          name_conflict     = 5
          db_access_error   = 6
          OTHERS            = 7.
      e_rc = sy-subrc.
      IF e_rc <> 0.
        MESSAGE ID sy-msgid TYPE 'S' NUMBER sy-msgno
          WITH sy-msgv1 sy-msgv2 sy-msgv3 sy-msgv4
          INTO lv_text.
        e_message = lv_text.
        RETURN.
      ENDIF.

      " RPY_TABLE_INSERT 已负责仓库登记与 CTS 归属；激活单独显式执行，
      " 且 AUTH_CHK 保持 'X'——成功决不意味着留下只存在于非激活状态的表。
      CALL FUNCTION 'DDIF_TABL_ACTIVATE'
        EXPORTING
          name     = i_table
          auth_chk = 'X'
        IMPORTING
          rc       = e_activation_rc
        EXCEPTIONS
          not_found   = 1
          put_failure = 2
          OTHERS      = 3.
      IF sy-subrc <> 0 OR e_activation_rc > 4.
        e_rc = 12.
        MESSAGE ID sy-msgid TYPE 'S' NUMBER sy-msgno
          WITH sy-msgv1 sy-msgv2 sy-msgv3 sy-msgv4
          INTO lv_text.
        e_message = |Table was created but activation failed: { lv_text }|.
      ENDIF.

    WHEN OTHERS.
      e_rc = 8.
      e_message = |Unsupported I_OP '{ i_op }'. Use TEXTPOOL_GET, TEXTPOOL_SET, or TABLE_CREATE|.
  ENDCASE.
ENDFUNCTION.
