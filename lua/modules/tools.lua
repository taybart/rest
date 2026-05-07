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
  local expect = rest.req.expect
  if expect == nil then return true end
  local res = rest.res
  if expect.status ~= res.status then
    if not dont_fail then
      fail(('unexpected status code %d != %d'):format(expect.status, res.status))
    end
    return false
  end
  if expect.body ~= nil and expect.body ~= '' and expect.body ~= res.body then
    if not dont_fail then
      fail(('body does not match expectation %s != %s'):format(expect.body, res.body))
    end
    return false
  end
  if expect.headers ~= nil then
    for k, v in pairs(expect.headers) do
      local values, finalkey = M.get_header_values(k)
      if values == nil then
        if not dont_fail then fail(('header %s does not present "%s" != "%s"'):format(finalkey)) end
        return false
      end
      local matches = false
      for _, value in ipairs(values) do
        if v == value then matches = true end
      end
      if not matches then
        -- small assumption that header is standalone for usablilty
        if not dont_fail then
          fail(('header %s does not match "%s" != "%s"'):format(finalkey, v, values[1]))
        end
        return false
      end
    end
  end
  return true
end

--[[
  returns the first value of the header and the key that matched
  will check for multiple keys to see if the header has weird capitalization
--]]
function M.get_res_header(_key) return M.get_header(_key) end

-- get authorization header and remove "Bearer "
function M.get_req_bearer_token()
  local auth_header = M.get_header('authorization', true)
  if auth_header == nil then return nil end
  return auth_header:gsub('Bearer ', '')
end

function M.get_req_header(_key) return M.get_header(_key, true) end

function M.get_header(_key, use_req)
  local header, key = M.get_header_values(_key, use_req)
  if header == nil then return nil, key end
  return header['1'], key
end

--[[
  returns the key that matched and all of the values of the header returned by the server
  will check for multiple keys to see if the header has weird capitalization
--]]
function M.get_header_values(key, use_req)
  local headers
  -- don't access res in case we are running in the server
  if use_req then
    headers = rest.req.headers
  else
    headers = rest.res.headers
  end
  if headers[key] ~= nil then return headers[key], key end
  local upperkey = key:upper()
  if headers[upperkey] ~= nil then return headers[upperkey], upperkey end
  local lowerkey = key:lower()
  if headers[lowerkey] ~= nil then return headers[lowerkey], lowerkey end
  local mixedkey = key:gsub("(%a)([%w_']*)", function(a, b) return string.upper(a) .. b end)
  if headers[mixedkey] ~= nil then return headers[mixedkey], mixedkey end
  return nil, key
end

return M
