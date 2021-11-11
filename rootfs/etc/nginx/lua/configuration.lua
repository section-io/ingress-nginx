local cjson = require("cjson.safe")

local io = io
local ngx = ngx
local tostring = tostring
local string = string
local table = table
local pairs = pairs

-- this is the Lua representation of Configuration struct in internal/ingress/types.go
local configuration_data = ngx.shared.configuration_data
local certificate_data = ngx.shared.certificate_data
local certificate_servers = ngx.shared.certificate_servers
local ocsp_response_cache = ngx.shared.ocsp_response_cache
local metrics_data = ngx.shared.metrics_data

local EMPTY_UID = "-1"

local _M = {}

function _M.get_backends_data()
  return configuration_data:get("backends")
end

function _M.get_general_data()
  return configuration_data:get("general")
end

function _M.get_raw_backends_last_synced_at()
  local raw_backends_last_synced_at = configuration_data:get("raw_backends_last_synced_at")
  if raw_backends_last_synced_at == nil then
    raw_backends_last_synced_at = 1
  end
  return raw_backends_last_synced_at
end

local function fetch_request_body()
  ngx.req.read_body()
  local body = ngx.req.get_body_data()

  if not body then
    -- request body might've been written to tmp file if body > client_body_buffer_size
    local file_name = ngx.req.get_body_file()
    local file = io.open(file_name, "rb")

    if not file then
      return nil
    end

    body = file:read("*all")
    file:close()
  end

  return body
end

local function get_pem_cert(hostname)
  local uid = certificate_servers:get(hostname)
  if not uid then
    return nil
  end

  return certificate_data:get(uid)
end

local function handle_servers()
  if ngx.var.request_method ~= "POST" then
    ngx.status = ngx.HTTP_BAD_REQUEST
    ngx.print("Only POST requests are allowed!")
    return
  end

  local raw_configuration = fetch_request_body()

  if raw_configuration then
     metrics_data:set("cert_last_bytes", string.len(raw_configuration))
  end

  local configuration, err = cjson.decode(raw_configuration)
  if not configuration then
    ngx.log(ngx.ERR, "could not parse configuration: ", err)
    ngx.status = ngx.HTTP_BAD_REQUEST
    return
  end

  local err_buf = {}

  local servers_count_success, servers_count_fail, servers_count_forcible = 0, 0, 0
  for server, uid in pairs(configuration.servers) do
    if uid == EMPTY_UID then
      -- notice that we do not delete certificate corresponding to this server
      -- this is because a certificate can be used by multiple servers/hostnames
      certificate_servers:delete(server)
    else
      local success, set_err, forcible = certificate_servers:set(server, uid)
      if not success then
        local err_msg = string.format("error setting certificate for %s: %s\n",
          server, tostring(set_err))
        table.insert(err_buf, err_msg)
        servers_count_fail = servers_count_fail + 1
      else
        servers_count_success = servers_count_success + 1
      end
      if forcible then
        servers_count_forcible = servers_count_forcible + 1
        metrics_data:incr("servers_overflow_total", 1, 0, 0)
        local msg = string.format("certificate_servers dictionary is full, "
          .. "LRU entry has been removed to store %s", server)
        ngx.log(ngx.WARN, msg)
      end
    end
  end
  metrics_data:set("servers_last_success", servers_count_success - servers_count_forcible)
  metrics_data:set("servers_last_fail", servers_count_fail)
  metrics_data:set("servers_last_forcible", servers_count_forcible)

  local cert_count_success, cert_count_fail, cert_count_forcible = 0, 0, 0
  for uid, cert in pairs(configuration.certificates) do
    -- don't delete the cache here, certificate_data[uid] is not replaced yet.
    -- there is small chance that nginx worker still get the old certificate,
    -- then fetch and cache the old OCSP Response
    local old_cert = certificate_data:get(uid)
    local is_renew = (old_cert ~= nil and old_cert ~= cert)

    local success, set_err, forcible = certificate_data:set(uid, cert)
    if success then
        -- delete ocsp cache after certificate_data:set succeed
        if is_renew then
            ocsp_response_cache:delete(uid)
        end
        cert_count_success = cert_count_success + 1
    else
      local err_msg = string.format("error setting certificate for %s: %s\n",
        uid, tostring(set_err))
      table.insert(err_buf, err_msg)
      cert_count_fail = cert_count_fail + 1
    end
    if forcible then
      cert_count_forcible = cert_count_forcible + 1
      metrics_data:incr("cert_overflow_total", 1, 0, 0)
      local msg = string.format("certificate_data dictionary is full, "
        .. "LRU entry has been removed to store %s", uid)
      ngx.log(ngx.WARN, msg)
    end
  end
  metrics_data:set("cert_last_success", cert_count_success - cert_count_forcible)
  metrics_data:set("cert_last_fail", cert_count_fail)
  metrics_data:set("cert_last_forcible", cert_count_forcible)

  if #err_buf > 0 then
    ngx.log(ngx.ERR, table.concat(err_buf))
    ngx.status = ngx.HTTP_INTERNAL_SERVER_ERROR
    return
  end

  ngx.status = ngx.HTTP_CREATED
end

local function handle_general()
  if ngx.var.request_method == "GET" then
    ngx.status = ngx.HTTP_OK
    ngx.print(_M.get_general_data())
    return
  end

  if ngx.var.request_method ~= "POST" then
    ngx.status = ngx.HTTP_BAD_REQUEST
    ngx.print("Only POST and GET requests are allowed!")
    return
  end

  local config = fetch_request_body()

  local success, err = configuration_data:safe_set("general", config)
  if not success then
    ngx.status = ngx.HTTP_INTERNAL_SERVER_ERROR
    ngx.log(ngx.ERR, "error setting general config: " .. tostring(err))
    return
  end

  ngx.status = ngx.HTTP_CREATED
end

local function handle_certs()
  if ngx.var.request_method ~= "GET" then
    ngx.status = ngx.HTTP_BAD_REQUEST
    ngx.print("Only GET requests are allowed!")
    return
  end

  local query = ngx.req.get_uri_args()
  if not query["hostname"] then
    ngx.status = ngx.HTTP_BAD_REQUEST
    ngx.print("Hostname must be specified.")
    return
  end

  local key = get_pem_cert(query["hostname"])
  if key then
    ngx.status = ngx.HTTP_OK
    ngx.print(key)
    return
  else
    ngx.status = ngx.HTTP_NOT_FOUND
    ngx.print("No key associated with this hostname.")
    return
  end
end

local function handle_metrics()
  ngx.status = ngx.HTTP_OK
  ngx.say("conf_free_space_bytes: ", configuration_data:free_space())
  ngx.say("conf_capacity_bytes: ", configuration_data:capacity())

  ngx.say("cert_last_bytes: ", metrics_data:get("cert_last_bytes") or 0)

  ngx.say("cert_capacity_bytes: ", certificate_data:capacity())
  ngx.say("cert_free_space_bytes: ", certificate_data:free_space())
  ngx.say("cert_last_success: ", metrics_data:get("cert_last_success") or 0)
  ngx.say("cert_last_fail: ", metrics_data:get("cert_last_fail") or 0)
  ngx.say("cert_last_forcible: ", metrics_data:get("cert_last_forcible") or 0)
  ngx.say("cert_overflow_total: ", metrics_data:get("cert_overflow_total") or 0)


  -- servers involve the lookup between a host and uid, the uid is
  -- then used to get the cert, since a cert could be shared between
  -- multiple hosts
  ngx.say("servers_capacity_bytes: ", certificate_servers:capacity())
  ngx.say("servers_free_space_bytes: ", certificate_servers:free_space())
  ngx.say("servers_last_fail: ", metrics_data:get("servers_last_fail") or 0)
  ngx.say("servers_last_forcible: ", metrics_data:get("servers_last_forcible") or 0)
  ngx.say("servers_last_success: ", metrics_data:get("servers_last_success") or 0)
  ngx.say("servers_overflow_total: ", metrics_data:get("servers_overflow_total") or 0)
end

local function handle_backends()
  if ngx.var.request_method == "GET" then
    ngx.status = ngx.HTTP_OK
    ngx.print(_M.get_backends_data())
    return
  end

  local backends = fetch_request_body()
  if not backends then
    ngx.log(ngx.ERR, "dynamic-configuration: unable to read valid request body")
    ngx.status = ngx.HTTP_BAD_REQUEST
    return
  end

  local success, err = configuration_data:set("backends", backends)
  if not success then
    ngx.log(ngx.ERR, "dynamic-configuration: error updating configuration: " .. tostring(err))
    ngx.status = ngx.HTTP_BAD_REQUEST
    return
  end

  ngx.update_time()
  local raw_backends_last_synced_at = ngx.time()
  success, err = configuration_data:set("raw_backends_last_synced_at", raw_backends_last_synced_at)
  if not success then
    ngx.log(ngx.ERR, "dynamic-configuration: error updating when backends sync, " ..
                     "new upstream peers waiting for force syncing: " .. tostring(err))
    ngx.status = ngx.HTTP_BAD_REQUEST
    return
  end

  ngx.status = ngx.HTTP_CREATED
end

function _M.call()
  if ngx.var.request_method ~= "POST" and ngx.var.request_method ~= "GET" then
    ngx.status = ngx.HTTP_BAD_REQUEST
    ngx.print("Only POST and GET requests are allowed!")
    return
  end

  if ngx.var.request_uri == "/configuration/servers" then
    handle_servers()
    return
  end

  if ngx.var.request_uri == "/configuration/general" then
    handle_general()
    return
  end

  if ngx.var.uri == "/configuration/certs" then
    handle_certs()
    return
  end

  if ngx.var.request_uri == "/configuration/metrics" then
    handle_metrics()
    return
  end

  if ngx.var.request_uri == "/configuration/backends" then
    handle_backends()
    return
  end

  ngx.status = ngx.HTTP_NOT_FOUND
  ngx.print("Not found!")
end

setmetatable(_M, {__index = { handle_servers = handle_servers }})

return _M
