local M = {}

--[[
 Checks wether the response matches the expect block, returns boolean
 This will call fail on its own so should be used like:
 if not tools.expected() then
  return
 end
 So that the error is correctly printed, if you don't want fail called,
 pass true: if not tools.expected(true) then fail('custom error') end
--]]
function M.expected(dont_fail)
  if __rest_check_expectation then
    local ok, err = __rest_check_expectation()
    if not ok then
      if not dont_fail then
        fail(err)
      end
      return false
    end
    return true
  end
  return true
end

local function get_path(tbl, path)
  if type(tbl) ~= 'table' or type(path) ~= 'string' then return nil end
  if tbl[path] ~= nil then return tbl[path] end
  local cur = tbl
  for part in path:gmatch('[^.]+') do
    if type(cur) ~= 'table' then return nil end
    local idx = tonumber(part)
    if idx ~= nil and cur[idx] ~= nil then
      cur = cur[idx]
    else
      cur = cur[part]
    end
  end
  return cur
end

local function normalize_keys(...)
  if select('#', ...) == 1 and type((...)) == 'table' then
    return (...)
  end
  return { ... }
end

local function select_fields(body, ...)
  local keys = normalize_keys(...)
  if next(keys) == nil then
    return body
  end
  local out = {}
  -- Array of keys: {"id", "name"}
  if #keys > 0 then
    for _, k in ipairs(keys) do
      out[k] = get_path(body, k)
    end
    return out
  end
  -- Map of alias = path: { user_id = "user.id" }
  for alias, path in pairs(keys) do
    out[alias] = get_path(body, path)
  end
  return out
end

function M.inspect_res(...)
  local body = json.decode(rest.res.body)
  inspect.print(select_fields(body, ...))
end

function M.inspect_req(...)
  local body = json.decode(rest.req.body)
  inspect.print(select_fields(body, ...))
end

function M.export_res(...)
  local body = json.decode(rest.res.body)
  local keys = normalize_keys(...)
  if #keys > 0 then
    for _, k in ipairs(keys) do
      rest.exports[k] = get_path(body, k)
    end
    return
  end
  for alias, path in pairs(keys) do
    rest.exports[alias] = get_path(body, path)
  end
end

function M.copy_res(key)
  local body = json.decode(rest.res.body)
  if key == nil or key == '' then
    copy(rest.res.body)
    return
  end
  copy(get_path(body, key))
end

local function find_jwt_token(key)
  if rest.res ~= nil and rest.res.body ~= nil and rest.res.body ~= '' then
    if jwt.is_valid(rest.res.body) then
      return rest.res.body
    end
    local ok, body = pcall(json.decode, rest.res.body)
    if ok and type(body) == 'table' then
      if type(key) == 'string' and key ~= '' then
        local val = get_path(body, key)
        if type(val) == 'string' then return val end
      else
        for _, candidate in ipairs({ 'token', 'access_token', 'jwt', 'id_token' }) do
          if type(body[candidate]) == 'string' and jwt.is_valid(body[candidate]) then
            return body[candidate]
          end
        end
      end
    end
  end
  if rest.req ~= nil and rest.req.bearer_token then
    local tok = rest.req:bearer_token()
    if tok and jwt.is_valid(tok) then return tok end
  end
  return nil
end

function M.inspect_jwt(key, ...)
  local tok = find_jwt_token(key)
  if not tok then
    print('no jwt token found')
    return
  end
  local claims, header = jwt.decode(tok)
  if select('#', ...) > 0 then
    inspect.print(select_fields(claims, ...))
    return
  end
  inspect.print({ header = header, claims = claims })
end

return M
