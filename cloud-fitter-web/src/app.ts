import { RequestConfig } from 'umi';
import { API_REQUEST_TIMEOUT_MS } from '@/constants/requestTimeout';
import { errorHandler } from '@/utils/errorHandle';

export const request: RequestConfig = {
  timeout: API_REQUEST_TIMEOUT_MS,
  // errorConfig: {
  //   adaptor: (res) => {
  //     return {
  //       success: res,
  //       data: res,
  //       errorCode: res,
  //       errorMessage: res,
  //     };
  //   },
  // },
  errorHandler,
  middlewares: [],
  requestInterceptors: [],
  responseInterceptors: [
    (response) => {
      return response;
    },
  ],
};
