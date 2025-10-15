local M = {}

--[[
 Checks wether the response matches the expect block, returns boolean
 This will call fail on its own so should be used like:
 if not tools.is_expected_response() then
  return
 end
 So that the error is correctly printed
--]]
function M.is_expected_response()
  local expect = rest.req.expect
  if expect == nil then
    return true
  end
  local res = rest.res
  if expect.status ~= res.status then
    fail(('unexpected status code %d != %d'):format(expect.status, res.status))
    return false
  end
  if expect.body ~= nil and expect.body ~= '' and expect.body ~= res.body then
    fail(('body does not match expectation %s != %s'):format(expect.body, res.body))
    return false
  end
  if expect.headers ~= nil then
    for k, v in pairs(expect.headers) do
      local values, finalkey = M.get_header_values(k)
      if values == nil then
        fail(('header %s does not present "%s" != "%s"'):format(finalkey))
        return false
      end
      local matches = false
      for _, value in ipairs(values) do
        if v == value then
          matches = true
        end
      end
      if not matches then
        -- small assumption that header is standalone for usablilty
        fail(('header %s does not match "%s" != "%s"'):format(finalkey, v, values[1]))
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
function M.get_res_header(_key)
  return M.get_header(_key)
end

function M.get_req_header(_key)
  return M.get_header(_key, true)
end

function M.get_header(_key, req)
  local header, key = M.get_header_values(_key, req)
  if header == nil then
    return nil, key
  end
  return header['1'], key
end

--[[
  returns the key that matched and all of the values of the header returned by the server
  will check for multiple keys to see if the header has weird capitalization
--]]
function M.get_header_values(key, req)
  local headers = rest.res.headers
  if req then
    headers = rest.req.headers
  end
  if headers[key] ~= nil then
    return headers[key], key
  end
  local upperkey = key:upper()
  if headers[upperkey] ~= nil then
    return headers[upperkey], upperkey
  end
  local lowerkey = key:lower()
  if headers[lowerkey] ~= nil then
    return headers[lowerkey], lowerkey
  end
  local mixedkey = key:gsub("(%a)([%w_']*)", function(a, b)
    return string.upper(a) .. b
  end)
  if headers[mixedkey] ~= nil then
    return headers[mixedkey], mixedkey
  end
  return nil, key
end

return M
